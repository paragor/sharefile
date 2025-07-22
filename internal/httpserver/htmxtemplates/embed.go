package htmxtemplates

import "embed"

//go:embed components/*
var Components embed.FS

//go:embed pages/*
var Pages embed.FS
