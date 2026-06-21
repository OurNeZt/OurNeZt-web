package web

import (
	"html/template"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/OurNeZt/ournezt-web/internal/config"
	"github.com/OurNeZt/ournezt-web/internal/core"
	"github.com/gin-gonic/gin"
)

type App struct {
	cfg     config.Config
	clients *core.Clients
}

type CurrentUser struct {
	ID                 string
	Email              string
	DisplayName        string
	Role               string
	MustChangePassword bool
}

type ViewData struct {
	Title string
	User  *CurrentUser
	Error string
	Flash string
	Data  any
}

const userContextKey = "current_user"
const tokenContextKey = "session_token"

func NewRouter(cfg config.Config, clients *core.Clients) (*gin.Engine, error) {
	app := &App{cfg: cfg, clients: clients}

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(app.sessionLoader())

	tpl, err := parseTemplates("templates")
	if err != nil {
		return nil, err
	}
	r.SetHTMLTemplate(tpl)
	r.Static("/static", "./static")

	r.GET("/", app.home)
	r.GET("/login", app.showLogin)
	r.GET("/bootstrap-admin-help", app.bootstrapAdminHelp)
	r.POST("/login", app.login)
	r.POST("/logout", app.logout)

	authed := r.Group("/")
	authed.Use(app.requireAuth())
	authed.GET("/faq", app.faq)
	authed.GET("/change-password", app.showChangePassword)
	authed.POST("/change-password", app.changePassword)

	protected := authed.Group("/")
	protected.Use(app.requirePasswordChanged())
	protected.GET("/dashboard", app.dashboard)

	admin := protected.Group("/admin")
	admin.Use(app.requireAdmin())
	admin.GET("", app.adminHome)
	admin.GET("/users", app.adminUsers)
	admin.POST("/users", app.adminCreateUser)
	admin.POST("/users/:id/disable", app.adminDisableUser)

	member := protected.Group("/")
	member.Use(app.requireStandardUser())
	member.GET("/families", app.families)
	member.GET("/families/new", app.newFamily)
	member.POST("/families", app.createFamily)
	member.GET("/families/:id", app.familyDetail)
	member.POST("/families/join", app.joinFamily)
	member.POST("/families/:id/invite-code/regenerate", app.regenerateFamilyCode)
	member.GET("/people", app.people)
	member.GET("/people/new", app.newPerson)
	member.POST("/people", app.createPerson)
	member.GET("/people/:id/edit", app.editPerson)
	member.POST("/people/:id", app.updatePerson)
	member.POST("/people/:id/delete", app.deletePerson)
	member.GET("/profile", app.profile)
	member.POST("/profile/account", app.profileUpdateAccount)
	member.POST("/profile/password", app.profileChangePassword)
	member.GET("/profile/person/:id/edit", app.profileEditSelfPerson)
	member.POST("/profile/person/:id", app.profileUpdateSelfPerson)
	member.GET("/profile/person/new", app.profileNewSelfPerson)
	member.POST("/profile/person", app.profileCreateSelfPerson)
	member.GET("/housing", app.housing)
	member.GET("/housing/new", app.newHousing)
	member.POST("/housing", app.createHousing)
	member.GET("/housing/:id", app.housingDetail)
	member.GET("/housing/:id/edit", app.editHousing)
	member.POST("/housing/:id", app.updateHousing)
	member.POST("/housing/:id/delete", app.deleteHousing)
	member.GET("/housing/compare", app.compareHousing)
	member.GET("/settings", app.settings)

	return r, nil
}

func parseTemplates(root string) (*template.Template, error) {
	paths := make([]string, 0, 32)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".tmpl" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	tpl := template.New("ournezt-web").Funcs(template.FuncMap{
		"cents":                       centsString,
		"moneyInput":                  centsInputString,
		"bpsInput":                    bpsInputString,
		"eqFold":                      eqFold,
		"ratingLabel":                 housingRatingLabel,
		"housingTooltip":              housingTooltip,
		"housingTooltipSupportingTip": housingTooltipSupportingTip,
		"housingTooltipSeeMoreLabel":  housingTooltipSeeMoreLabel,
		"housingTooltipSeeMoreURL":    housingTooltipSeeMoreURL,
		"tooltipAriaLabel":            tooltipAriaLabel,
		"dict":                        dict,
		"monthsToYears":               monthsToYears,
	})
	return tpl.ParseFiles(paths...)
}
