package web

import (
	"errors"
	"net/http"
	"strings"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
)

type loginPayload struct{}
type changePasswordPayload struct{}
type bootstrapAdminHelpPayload struct{}

type profilePersonShortcut struct {
	FamilyID     string
	FamilyName   string
	PersonID     string
	PersonName   string
	Relationship string
}

type profilePayload struct {
	User         *CurrentUser
	Families     []*ourneztv1.Family
	SelfProfiles []profilePersonShortcut
}

func (a *App) home(c *gin.Context) {
	user := userFromContext(c)
	if user != nil {
		if user.MustChangePassword {
			c.Redirect(http.StatusFound, "/change-password")
			return
		}
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	c.Redirect(http.StatusFound, "/login")
}

func (a *App) showLogin(c *gin.Context) {
	user := userFromContext(c)
	if user != nil {
		if user.MustChangePassword {
			c.Redirect(http.StatusFound, "/change-password")
			return
		}
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	a.render(c, "login", "Login", loginPayload{})
}

func (a *App) bootstrapAdminHelp(c *gin.Context) {
	a.render(c, "bootstrap_admin_help", "Bootstrap Admin Help", bootstrapAdminHelpPayload{})
}

func (a *App) login(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")
	if email == "" || password == "" {
		c.Redirect(http.StatusFound, "/login?error=Email+and+password+are+required")
		return
	}

	resp, err := a.clients.Auth.Login(a.grpcContext(c), &ourneztv1.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/login?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	http.SetCookie(c.Writer, sessionCookie(a.cfg.SessionCookieName, resp.GetSessionToken(), a.cfg.SessionCookieMax, a.cfg.CookieSecure))
	if resp.GetUser().GetMustChangePassword() {
		c.Redirect(http.StatusFound, "/change-password?flash=Please+set+a+new+password")
		return
	}
	c.Redirect(http.StatusFound, "/dashboard?flash=Welcome")
}

func (a *App) logout(c *gin.Context) {
	http.SetCookie(c.Writer, clearSessionCookie(a.cfg.SessionCookieName, a.cfg.CookieSecure))
	c.Redirect(http.StatusFound, "/login?flash=Logged+out")
}

func (a *App) settings(c *gin.Context) {
	c.Redirect(http.StatusFound, "/profile")
}

func (a *App) showChangePassword(c *gin.Context) {
	if userFromContext(c) == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}
	a.render(c, "change_password", "Change Password", changePasswordPayload{})
}

func (a *App) changePassword(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	if err := a.changePasswordFromPost(c); err != nil {
		c.Redirect(http.StatusFound, "/change-password?error="+urlQuerySafe(err.Error()))
		return
	}

	c.Redirect(http.StatusFound, "/dashboard?flash=Password+updated")
}

func (a *App) profile(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	families, selfProfiles := a.loadProfileFamiliesAndSelfProfiles(c, user)

	a.render(c, "profile", "My Profile", profilePayload{
		User:         user,
		Families:     families,
		SelfProfiles: selfProfiles,
	})
}

func (a *App) profileUpdateAccount(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	email := strings.TrimSpace(c.PostForm("email"))
	displayName := strings.TrimSpace(c.PostForm("display_name"))
	if email == "" || displayName == "" {
		c.Redirect(http.StatusFound, "/profile?error=Email+and+display+name+are+required")
		return
	}

	_, err := a.clients.Auth.UpdateMyAccount(a.grpcContext(c), &ourneztv1.UpdateMyAccountRequest{
		Email:       email,
		DisplayName: displayName,
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	c.Redirect(http.StatusFound, "/profile?flash=Account+updated")
}

func (a *App) profileChangePassword(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	if err := a.changePasswordFromPost(c); err != nil {
		c.Redirect(http.StatusFound, "/profile?error="+urlQuerySafe(err.Error()))
		return
	}

	c.Redirect(http.StatusFound, "/profile?flash=Password+updated")
}

func (a *App) profileEditSelfPerson(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	resp, err := a.clients.Person.GetPersonProfile(a.grpcContext(c), &ourneztv1.GetPersonProfileRequest{
		ViewerUserId: user.ID,
		PersonId:     c.Param("id"),
	})
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	if !matchesCurrentUserProfile(user, resp) {
		c.Redirect(http.StatusFound, "/people?family_id="+resp.GetFamilyId()+"&error=This+profile+is+managed+from+People")
		return
	}

	a.render(c, "person_form", "Edit My Financial Profile", personFormData{
		FamilyID:       resp.GetFamilyId(),
		Person:         resp,
		IsEdit:         true,
		ManagedByOwner: false,
		ReturnTo:       "/profile",
		FormAction:     "/profile/person/" + resp.GetId(),
	})
}

func (a *App) profileUpdateSelfPerson(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	personID := c.Param("id")
	current, currentErr := a.clients.Person.GetPersonProfile(a.grpcContext(c), &ourneztv1.GetPersonProfileRequest{
		ViewerUserId: user.ID,
		PersonId:     personID,
	})
	if currentErr != nil {
		c.Redirect(http.StatusFound, "/profile?error="+urlQuerySafe(grpcMessage(currentErr)))
		return
	}
	if !matchesCurrentUserProfile(user, current) {
		c.Redirect(http.StatusFound, "/people?family_id="+current.GetFamilyId()+"&error=This+profile+is+managed+from+People")
		return
	}

	person := personFromForm(c)
	person.Id = personID
	person.FamilyId = current.GetFamilyId()
	if strings.TrimSpace(current.GetLinkedUserId()) != "" {
		person.LinkedUserId = current.GetLinkedUserId()
	} else {
		person.LinkedUserId = user.ID
	}
	if validationErr := validatePersonProfileInput(person); validationErr != "" {
		c.Redirect(http.StatusFound, "/profile/person/"+personID+"/edit?error="+urlQuerySafe(validationErr))
		return
	}
	_, err := a.clients.Person.UpdatePersonProfile(a.grpcContext(c), person)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile/person/"+personID+"/edit?error="+urlQuerySafe(grpcMessage(err)))
		return
	}
	c.Redirect(http.StatusFound, "/profile?flash=Profile+updated")
}

func (a *App) profileNewSelfPerson(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	familyID := strings.TrimSpace(c.Query("family_id"))
	if familyID == "" {
		c.Redirect(http.StatusFound, "/profile?error=Choose+a+family+first")
		return
	}

	a.render(c, "person_form", "Create My Financial Profile", personFormData{
		FamilyID: familyID,
		Person: &ourneztv1.PersonProfile{
			Name: user.DisplayName,
		},
		ManagedByOwner: false,
		ReturnTo:       "/profile",
		FormAction:     "/profile/person",
	})
}

func (a *App) profileCreateSelfPerson(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}

	person := personFromForm(c)
	person.Id = ""
	person.FamilyId = strings.TrimSpace(c.PostForm("family_id"))
	person.LinkedUserId = user.ID
	if person.GetFamilyId() == "" {
		c.Redirect(http.StatusFound, "/profile?error=Family+is+required")
		return
	}
	if strings.TrimSpace(person.GetName()) == "" {
		person.Name = user.DisplayName
	}
	if validationErr := validatePersonProfileInput(person); validationErr != "" {
		c.Redirect(http.StatusFound, "/profile/person/new?family_id="+person.GetFamilyId()+"&error="+urlQuerySafe(validationErr))
		return
	}

	_, err := a.clients.Person.CreatePersonProfile(a.grpcContext(c), person)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile/person/new?family_id="+person.GetFamilyId()+"&error="+urlQuerySafe(grpcMessage(err)))
		return
	}

	c.Redirect(http.StatusFound, "/profile?flash=Profile+created")
}

func (a *App) loadProfileFamiliesAndSelfProfiles(c *gin.Context, user *CurrentUser) ([]*ourneztv1.Family, []profilePersonShortcut) {
	if user == nil {
		return nil, nil
	}

	familyResp, familyErr := a.clients.Family.ListUserFamilies(a.grpcContext(c), &ourneztv1.ListUserFamiliesRequest{
		UserId: user.ID,
	})
	if familyErr != nil {
		return nil, nil
	}

	families := familyResp.GetFamilies()
	selfProfiles := make([]profilePersonShortcut, 0)

	for _, family := range families {
		peopleResp, peopleErr := a.clients.Person.ListPersonProfilesByFamily(a.grpcContext(c), &ourneztv1.ListPersonProfilesByFamilyRequest{
			ViewerUserId: user.ID,
			FamilyId:     family.GetId(),
		})
		if peopleErr != nil {
			continue
		}
		for _, person := range peopleResp.GetPeople() {
			if matchesCurrentUserProfile(user, person) {
				selfProfiles = append(selfProfiles, profilePersonShortcut{
					FamilyID:     family.GetId(),
					FamilyName:   family.GetName(),
					PersonID:     person.GetId(),
					PersonName:   person.GetName(),
					Relationship: person.GetRelationshipLabel(),
				})
			}
		}
	}

	return families, selfProfiles
}

func (a *App) changePasswordFromPost(c *gin.Context) error {
	currentPassword := strings.TrimSpace(c.PostForm("current_password"))
	newPassword := strings.TrimSpace(c.PostForm("new_password"))
	confirmPassword := strings.TrimSpace(c.PostForm("confirm_new_password"))
	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		return errors.New("all password fields are required")
	}
	if newPassword != confirmPassword {
		return errors.New("new password confirmation does not match")
	}

	_, err := a.clients.Auth.ChangePassword(a.grpcContext(c), &ourneztv1.ChangePasswordRequest{
		CurrentPassword: currentPassword,
		NewPassword:     newPassword,
	})
	if err != nil {
		return errors.New(grpcMessage(err))
	}
	return nil
}

func matchesCurrentUserProfile(user *CurrentUser, person *ourneztv1.PersonProfile) bool {
	if user == nil || person == nil {
		return false
	}
	if strings.TrimSpace(person.GetLinkedUserId()) != "" && strings.EqualFold(strings.TrimSpace(person.GetLinkedUserId()), strings.TrimSpace(user.ID)) {
		return true
	}

	userName := normalizeLookup(user.DisplayName)
	personName := normalizeLookup(person.GetName())
	rel := normalizeLookup(person.GetRelationshipLabel())
	return (userName != "" && userName == personName) || rel == "self" || rel == "me"
}
