// Package rest provides HTTP REST API endpoints and Swagger UI for WSBroker Display.
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/wsbrokerdisplay/data"
	"github.com/je4/wsbrokerdisplay/pkg/rest/docs"
	"go.ub.unibas.ch/cloud/swaggerui"
)

// BASEPATH defines the base URL prefix for version 1 of the WSBroker Display REST API.
const BASEPATH = "/api/v1"

// DataProvider is a function that retrieves a fresh snapshot of the dashboard telemetry and metadata.
type DataProvider func() DashboardData

// Option defines a functional configuration option for the REST controller.
type Option func(ctrl *controller)

// WithDataProvider configures the custom data provider function for telemetry and status endpoints.
func WithDataProvider(provider DataProvider) Option {
	return func(ctrl *controller) {
		ctrl.dataProvider = provider
	}
}

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
func NewController(addr, extAddr string, logger zLogger.ZLogger, opts ...Option) (*controller, error) {
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
		addr:      addr,
		extAddr:   extAddr,
		router:    router,
		subpath:   subpath,
		logger:    logger,
		startTime: time.Now().UTC(),
	}

	for _, opt := range opts {
		opt(c)
	}

	if err := c.Init(); err != nil {
		return nil, errors.Wrap(err, "cannot initialize rest controller")
	}
	return c, nil
}

// controller manages the HTTP router and server lifecycle.
type controller struct {
	server       http.Server
	router       *gin.Engine
	addr         string
	extAddr      string
	subpath      string
	logger       zLogger.ZLogger
	dataProvider DataProvider
	startTime    time.Time
}

// SetDataProvider updates the telemetry data provider.
func (ctrl *controller) SetDataProvider(provider DataProvider) {
	ctrl.dataProvider = provider
}

// GetRouter returns the underlying Gin Engine instance.
func (ctrl *controller) GetRouter() *gin.Engine {
	return ctrl.router
}

// Init configures middleware, REST endpoints, Swagger UI, static assets, templates, and the HTTP server.
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

	// Mount embedded static assets on /static/*
	subStatic, err := fs.Sub(data.WebFS, "web/static")
	if err != nil {
		return errors.Wrap(err, "cannot get sub filesystem for static assets")
	}
	ctrl.router.StaticFS("/static", http.FS(subStatic))

	// Load embedded HTML templates
	tmpl, err := template.ParseFS(data.WebFS, "web/templates/*.gohtml")
	if err != nil {
		return errors.Wrap(err, "cannot parse embedded web templates")
	}
	ctrl.router.SetHTMLTemplate(tmpl)

	// Web Dashboard routes
	ctrl.router.GET("/", ctrl.dashboard)
	ctrl.router.GET("/dashboard", ctrl.dashboard)
	ctrl.router.GET("/dashboard/cards", ctrl.dashboardCards)

	// API v1 routes
	v1 := ctrl.router.Group(BASEPATH)
	v1.GET("/ping", ctrl.ping)
	v1.GET("/status", ctrl.status)
	v1.GET("/ntp", ctrl.ntpStatus)

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

func (ctrl *controller) getDashboardData() DashboardData {
	if ctrl.dataProvider != nil {
		return ctrl.dataProvider()
	}

	hostname, _ := os.Hostname()
	uptime := time.Since(ctrl.startTime).Truncate(time.Second).String()

	return DashboardData{
		Addr:      ctrl.addr,
		ExtAddr:   ctrl.extAddr,
		Hostname:  hostname,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		Uptime:    uptime,
		StartTime: ctrl.startTime,
	}
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
