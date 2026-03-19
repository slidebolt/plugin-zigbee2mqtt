package main

import (
	runtime "github.com/slidebolt/sb-runtime"

	"github.com/slidebolt/plugin-zigbee2mqtt/app"
)

func main() {
	runtime.Run(app.New())
}
