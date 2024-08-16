package realmadmin

//go:generate api/vgsocket/cleanup.sh ||:
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest validate api/vgsocket/swagger.yaml
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate server --regenerate-configureapi -P models.Principal -t api/vgsocket/gen-server -f api/vgsocket/swagger.yaml --exclude-main -A VGSocketRealm
//go:generate go run github.com/go-swagger/go-swagger/cmd/swagger@latest generate client -t api/vgsocket/gen-client -f api/vgsocket/swagger.yaml -A VGSocketRealm
//go:generate go mod tidy
