module github.com/MaxPastukhov15/Liquidity_platform/backend

go 1.26.0

toolchain go1.27.2

require (
	github.com/getkin/kin-openapi v0.142.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/oapi-codegen/nethttp-middleware v1.2.0
	github.com/oapi-codegen/runtime v1.7.0
	github.com/shopspring/decimal v1.6.0
	golang.org/x/crypto v0.58.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/gorilla/mux v1.8.1 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	// oapi-codegen подключён только директивой tool (ниже): go mod tidy держит его здесь с // indirect,
	// перенос в прямые зависимости tidy откатит, и CI-проверка tidy -diff упадёт.
	github.com/oapi-codegen/oapi-codegen/v2 v2.8.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/speakeasy-api/openapi v1.24.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
