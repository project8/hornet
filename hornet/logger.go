/*
* logger.go
*
* Logging functions with severity & slack messaging
*
*/

package hornet

import (
	"os"

	"github.com/op/go-logging"
)

// global logger
var Log = logging.MustGetLogger("hornet")
var format = logging.MustStringFormatter(
    "%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}%{color:reset}",
)

func InitializeLogging() {
	backend := logging.NewLogBackend(os.Stdout, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, format)
	logging.SetBackend(backendFormatter)
}
