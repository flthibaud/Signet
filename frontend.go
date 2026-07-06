package signet

import "embed"

//go:embed frontend/build/client/*
var FrontendDist embed.FS
