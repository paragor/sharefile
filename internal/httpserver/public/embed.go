package public

import "embed"

//go:embed static/*
var Static embed.FS
