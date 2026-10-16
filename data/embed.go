package data

import "embed"

// WebFS contains all embedded web templates and static assets for the dashboard.
//
//go:embed web/*
var WebFS embed.FS
