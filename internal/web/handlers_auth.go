package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/OurNeZt/ournezt-web/internal/authlimit"
	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

type loginPayload struct{}

const loginFailureMessage = "Invalid email or password."

type changePasswordPayload struct {
	IsAdmin  bool
	IsForced bool
}
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
		c.Redirect(http.StatusFound, "/login?error="+urlQuerySafe(loginFailureMessage))
		return
	}

	resp, err := a.clients.Auth.Login(a.grpcContext(c), &ourneztv1.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		if grpcCode(err) == codes.ResourceExhausted {
			rejectAuthTraffic(c, authRetryDelay(err))
			return
		}
		c.Redirect(http.StatusFound, "/login?error="+urlQuerySafe(loginFailureMessage))
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
	c.Header("Cache-Control", "no-store")
	if cookie, err := c.Request.Cookie(a.cfg.SessionCookieName); err == nil && cookie.Value != "" {
		// The token is only used to revoke this session, even if it is no longer valid.
		ctx := metadata.AppendToOutgoingContext(c.Request.Context(), "x-session-token", cookie.Value)
		if _, err := a.clients.Auth.RevokeCurrentSession(ctx, &ourneztv1.RevokeCurrentSessionRequest{}); err != nil && grpcCode(err) != codes.Unauthenticated {
			if !a.renderCoreFailure(c, grpcCode(err)) {
				a.renderCoreFailure(c, codes.Unavailable)
			}
			return
		}
	}
	http.SetCookie(c.Writer, clearSessionCookie(a.cfg.SessionCookieName, a.cfg.CookieSecure))
	c.Redirect(http.StatusFound, "/login?flash=Logged+out")
}

func (a *App) settings(c *gin.Context) {
	c.Redirect(http.StatusFound, "/profile")
}

func (a *App) showChangePassword(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}
	isAdmin := strings.EqualFold(strings.TrimSpace(user.Role), "admin")
	if !user.MustChangePassword && !isAdmin {
		c.Redirect(http.StatusFound, "/profile?error=Please+change+password+from+My+Profile")
		return
	}
	a.render(c, "change_password", "Change Password", changePasswordPayload{
		IsAdmin:  isAdmin,
		IsForced: user.MustChangePassword,
	})
}

func (a *App) changePassword(c *gin.Context) {
	user := userFromContext(c)
	if user == nil {
		c.Redirect(http.StatusFound, "/login?error=Please+log+in")
		return
	}
	isAdmin := strings.EqualFold(strings.TrimSpace(user.Role), "admin")
	if !user.MustChangePassword && !isAdmin {
		c.Redirect(http.StatusFound, "/profile?error=Please+change+password+from+My+Profile")
		return
	}

	if err := a.changePasswordFromPost(c); err != nil {
		var limited *authlimit.LimitError
		if errors.As(err, &limited) {
			rejectAuthTraffic(c, limited.RetryAfter)
			return
		}
		c.Redirect(http.StatusFound, "/change-password?error="+urlQuerySafe(err.Error()))
		return
	}
	a.passwordChanged(c)
}

// Core confirms password update and all-session revocation in the same operation.
// Clear the browser session only after that operation succeeds.
func (a *App) passwordChanged(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	http.SetCookie(c.Writer, clearSessionCookie(a.cfg.SessionCookieName, a.cfg.CookieSecure))
	c.Redirect(http.StatusFound, "/login?flash=Password+updated.+Please+log+in+again.")
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
		var limited *authlimit.LimitError
		if errors.As(err, &limited) {
			rejectAuthTraffic(c, limited.RetryAfter)
			return
		}
		c.Redirect(http.StatusFound, "/profile?error="+urlQuerySafe(err.Error()))
		return
	}

	a.passwordChanged(c)
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
	if linked := normalizeOptionalUUID(current.GetLinkedUserId()); linked != "" {
		person.LinkedUserId = linked
	} else {
		person.LinkedUserId = normalizeOptionalUUID(user.ID)
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
	person.LinkedUserId = normalizeOptionalUUID(user.ID)
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
	currentPassword := c.PostForm("current_password")
	newPassword := c.PostForm("new_password")
	confirmPassword := c.PostForm("confirm_new_password")
	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		return errors.New("all password fields are required")
	}
	if err := validateNewPasswordWhitespace(newPassword); err != nil {
		return err
	}
	if newPassword != confirmPassword {
		return errors.New("new password confirmation does not match")
	}

	_, err := a.clients.Auth.ChangePassword(a.grpcContext(c), &ourneztv1.ChangePasswordRequest{
		CurrentPassword: currentPassword,
		NewPassword:     newPassword,
	})
	if err != nil {
		if grpcCode(err) == codes.ResourceExhausted {
			return &authlimit.LimitError{RetryAfter: authRetryDelay(err)}
		}
		switch grpcCode(err) {
		case codes.InvalidArgument:
			return passwordValidationError(err)
		case codes.Unauthenticated:
			return errors.New("current password is incorrect or your session has expired")
		case codes.DeadlineExceeded, codes.Unavailable, codes.Canceled:
			return errors.New(grpcMessage(err))
		default:
			return errors.New("unable to change your password; please try again later")
		}
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
