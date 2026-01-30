package omnivore

import "embed"

//go:embed frontend/dist/*
var FrontendDist embed.FS
