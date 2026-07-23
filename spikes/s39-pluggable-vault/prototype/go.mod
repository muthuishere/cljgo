// Spike S39 throwaway module — never merges into pkg/ (ADR 0027 §5).
module cljgospike/s39

go 1.26

require (
	filippo.io/age v1.3.1
	github.com/zalando/go-keyring v0.2.8
)

require (
	filippo.io/hpke v0.4.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
)
