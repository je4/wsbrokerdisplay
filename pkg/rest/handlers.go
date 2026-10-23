// Package rest provides HTTP REST API endpoints and Swagger UI for WSBroker Display.
package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	_ "github.com/je4/wsbrokerclient/pkg/ntp"
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

// dashboard renders the full HTML dashboard landing page.
func (ctrl *controller) dashboard(c *gin.Context) {
	data := ctrl.getDashboardData()
	c.HTML(http.StatusOK, "dashboard.gohtml", data)
}

// dashboardCards renders the partial HTML cards for HTMX dynamic polling updates.
func (ctrl *controller) dashboardCards(c *gin.Context) {
	data := ctrl.getDashboardData()
	c.HTML(http.StatusOK, "cards.gohtml", data)
}

// status godoc
//
//	@Summary		get display status and system metadata
//	@ID				get-status
//	@Description	returns comprehensive dashboard telemetry, metadata, and NTP synchronization status
//	@Tags			wsbrokerdisplay
//	@Produce		json
//	@Success		200		{object}	DashboardData
//	@Router			/status [get]
func (ctrl *controller) status(c *gin.Context) {
	data := ctrl.getDashboardData()
	c.JSON(http.StatusOK, data)
}

// ntpStatus godoc
//
//	@Summary		get NTP synchronization status
//	@ID				get-ntp
//	@Description	returns live NTP synchronization metrics and clock offset
//	@Tags			wsbrokerdisplay
//	@Produce		json
//	@Success		200		{object}	ntp.NTPStatus
//	@Router			/ntp [get]
func (ctrl *controller) ntpStatus(c *gin.Context) {
	data := ctrl.getDashboardData()
	c.JSON(http.StatusOK, data.NTP)
}

// messages godoc
//
//	@Summary		get recent WebSocket message log
//	@ID				get-messages
//	@Description	returns rolling buffer of recent incoming and outgoing WebSocket messages
//	@Tags			wsbrokerdisplay
//	@Produce		json
//	@Success		200		{array}		WSMessage
//	@Router			/messages [get]
func (ctrl *controller) messages(c *gin.Context) {
	data := ctrl.getDashboardData()
	if data.Messages == nil {
		data.Messages = []WSMessage{}
	}
	c.JSON(http.StatusOK, data.Messages)
}
