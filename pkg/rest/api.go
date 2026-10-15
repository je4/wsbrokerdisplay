// Package rest provides HTTP REST API endpoints and Swagger UI for WSBroker Display.
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"emperror.dev/errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/wsbrokerdisplay/pkg/rest/docs"
	"go.ub.unibas.ch/cloud/swaggerui"
)

// BASEPATH defines the base URL prefix for version 1 of the WSBroker Display REST API.
const BASEPATH = "/api/v1"

//	@title			WSBroker Display API
//	@version		1.0
//	@description	WSBroker Display REST API
//	@termsOfService	http://swagger.io/terms/

//	@servers.url			/api/v1
//	@servers.description	WSBroker Display API base path

//	@contact.name	Jürgen Enge
//	@contact.url	https://ub.unibas.ch
//	@contact.email	juergen.enge@unibas.ch

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

// NewController creates and configures a new REST controller instance.
func NewController(addr, extAddr string, logger zLogger.ZLogger) (*controller, error) {
	u, err := url.Parse(extAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid external address '%s'", extAddr)
	}
	subpath := "/" + strings.Trim(u.Path, "/")

	// programmatically set swagger info
	docs.SwaggerInfoWSBrokerDisplay.Host = strings.TrimRight(fmt.Sprintf("%s:%s", u.Hostname(), u.Port()), " :")
	docs.SwaggerInfoWSBrokerDisplay.BasePath = "/" + strings.Trim(subpath+BASEPATH, "/")
	if u.Scheme != "" {
		docs.SwaggerInfoWSBrokerDisplay.Schemes = []string{u.Scheme}
	} else {
		docs.SwaggerInfoWSBrokerDisplay.Schemes = []string{"http"}
	}

	router := gin.Default()

	c := &controller{
		addr:    addr,
		router:  router,
		subpath: subpath,
		logger:  logger,
	}
	if err := c.Init(); err != nil {
		return nil, errors.Wrap(err, "cannot initialize rest controller")
	}
	return c, nil
}

// controller manages the HTTP router and server lifecycle.
type controller struct {
	server  http.Server
	router  *gin.Engine
	addr    string
	subpath string
	logger  zLogger.ZLogger
}

// GetRouter returns the underlying Gin Engine instance.
func (ctrl *controller) GetRouter() *gin.Engine {
	return ctrl.router
}

// Init configures middleware, REST endpoints, Swagger UI, and the HTTP server.
func (ctrl *controller) Init() error {
	ctrl.router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"*"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
	}))

	// Enforce that requests are received only via localhost
	ctrl.router.Use(func(c *gin.Context) {
		clientIP := c.ClientIP()
		ip := net.ParseIP(clientIP)
		if ip == nil || !ip.IsLoopback() {
			c.AbortWithStatusJSON(http.StatusForbidden, HTTPResultMessage{
				Code:    http.StatusForbidden,
				Message: "forbidden: access restricted to localhost",
			})
			return
		}
		c.Next()
	})

	v1 := ctrl.router.Group(BASEPATH)
	v1.GET("/ping", ctrl.ping)

	ctrl.router.GET("/swagger/*any", swaggerui.CustomHandler(
		swaggerui.WithTitle("WSBroker Display API"),
		swaggerui.WithDocFunc(func() string {
			doc := docs.SwaggerInfoWSBrokerDisplay.ReadDoc()
			if docs.SwaggerInfoWSBrokerDisplay.BasePath != "" {
				var spec map[string]any
				if err := json.Unmarshal([]byte(doc), &spec); err == nil {
					spec["servers"] = []map[string]any{
						{
							"url":         docs.SwaggerInfoWSBrokerDisplay.BasePath,
							"description": "WSBroker Display API base path",
						},
					}
					if b, err := json.Marshal(spec); err == nil {
						return string(b)
					}
				}
			}
			return doc
		}),
	))

	ctrl.server = http.Server{
		Addr:    ctrl.addr,
		Handler: ctrl.router,
	}

	return nil
}

// Start begins serving HTTP requests in a background goroutine.
func (ctrl *controller) Start(wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		fmt.Printf("starting server at http://%s\n", ctrl.addr)
		if err := ctrl.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server on '%s' ended: %v\n", ctrl.addr, err)
		}
	}()
}

// GracefulStop shuts down the HTTP server gracefully.
func (ctrl *controller) GracefulStop() {
	if err := ctrl.server.Shutdown(context.Background()); err != nil {
		if ctrl.logger != nil {
			ctrl.logger.Error().Err(err).Msg("error shutting down server")
		}
		return
	}
}
