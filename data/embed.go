package data

import "embed"

// WebFS contains all embedded web templates and static assets for the dashboard.
//
//go:embed web/static/css/* web/static/img/* web/static/js/* web/static/scss/*
//go:embed web/templates/*
var WebFS embed.FS
