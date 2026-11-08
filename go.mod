module github.com/omareloui/tyche

go 1.25.0

require github.com/a-h/templ v0.3.1020

require (
	github.com/almerlucke/go-iban v0.0.0-20220324081643-09bcab81b879
	github.com/go-playground/validator/v10 v10.22.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/joho/godotenv v1.5.1
	github.com/leebenson/conform v1.2.2
	github.com/oklog/ulid v1.3.1
	github.com/pressly/goose/v3 v3.24.1
	go.uber.org/zap v1.27.0
	golang.org/x/net v0.57.0
	golang.org/x/oauth2 v0.25.0
	golang.org/x/text v0.41.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/SherClockHolmes/webpush-go v1.4.0
	github.com/gen2brain/heic v0.7.2
	github.com/gen2brain/webp v0.6.4
	golang.org/x/image v0.45.0
)

require (
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
)

require (
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/webauthn v0.17.4
	github.com/go-webauthn/x v0.2.6 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
)

require (
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	github.com/a-h/parse v0.0.0-20250122154542-74294addb73e // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cli/browser v1.3.0 // indirect
	github.com/etgryphon/stringUp v0.0.0-20121020160746-31534ccd8cac // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.23
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/natefinch/atomic v1.0.1 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.54.0
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
)

tool github.com/a-h/templ/cmd/templ

replace github.com/mattn/go-sqlite3 => github.com/jgiannuzzi/go-sqlite3 v1.14.17-0.20230327162135-f208443ec79d
