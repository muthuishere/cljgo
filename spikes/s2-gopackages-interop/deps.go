// Package s2 pins interop target modules into go.mod so go/packages can
// resolve them (stand-in for the deps.edn -> go.mod pinning flow, doc 05 §1).
package s2

import (
	_ "github.com/google/uuid"
	_ "github.com/gorilla/websocket"
)
