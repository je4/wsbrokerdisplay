// Package rest provides HTTP REST API endpoints and Swagger UI for WSBroker Display.
package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ping godoc
//
//	@Summary		does pong
//	@ID				get-ping
//	@Description	for testing if server is running
//	@Tags			wsbrokerdisplay
//	@Produce		plain
//	@Success		200		{string}	string
//	@Router			/ping [get]
func (ctrl *controller) ping(c *gin.Context) {
	c.String(http.StatusOK, "pong")
}
