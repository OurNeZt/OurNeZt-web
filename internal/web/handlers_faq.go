package web

import "github.com/gin-gonic/gin"

func (a *App) faq(c *gin.Context) {
	a.render(c, "faq", "Housing Planning FAQ", struct{}{})
}
