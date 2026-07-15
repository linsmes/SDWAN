module sdwan/agent

go 1.26.0

require (
	golang.org/x/crypto v0.37.0
	golang.org/x/net v0.39.0
	golang.org/x/sys v0.32.0
	golang.zx2c4.com/wireguard v0.0.0-00010101000000-000000000000
)

require golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect

replace golang.zx2c4.com/wireguard => ../wireguard-go
