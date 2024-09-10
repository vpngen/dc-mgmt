package realmadmin

//go:generate api/vgsocket/cleanup.sh ||:
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest validate api/vgsocket/swagger.yaml
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate server --regenerate-configureapi -P models.Principal -t api/vgsocket/gen-server -f api/vgsocket/swagger.yaml --exclude-main -A VGSocketRealm
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate client -t api/vgsocket/gen-client -f api/vgsocket/swagger.yaml -A VGSocketRealm
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate cli -t api/vgsocket/gen-cli -f api/vgsocket/swagger.yaml -A VGSocketRealm
//go:generate go mod tidy

//go:generate api/vgsbrigades/cleanup.sh ||:
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest validate api/vgsbrigades/swagger.yaml
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate server --regenerate-configureapi -P models.Principal -t api/vgsbrigades/gen-server -f api/vgsbrigades/swagger.yaml --exclude-main -A VGSBrigadeRealm
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate client -t api/vgsbrigades/gen-client -f api/vgsbrigades/swagger.yaml -A VGSBrigadeRealm
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate cli -t api/vgsbrigades/gen-cli -f api/vgsbrigades/swagger.yaml -A VGSBrigadeRealm
//go:generate go mod tidy
