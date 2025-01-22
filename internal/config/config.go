package config

import "fmt"

func GetApplicationPort() int {
	return getEnvironmentInt("PORT")
}

func GetDataSource() string {
	return getEnvironmentValue("DATA_SOURCE")
}

func GetAppEnv() string {
	return getEnvironmentValueFromOptions("APP_ENV", []string{"development", "production"})
}

func GetLogLevel() int {
	return getEnvironmentIntWithDefault("LOG_LEVEL", 0)
}

func GetOAuthCBEndpoint() string {
	domain := getEnvironmentValue("OAUTH_CB_ORIGIN")
	return fmt.Sprintf("%s/oauth/cb", domain)
}

func GetGoogleOAuthClientID() string {
	return getEnvironmentValue("GOOGLE_CLIENT_ID")
}

func GetGoogleOAuthClientSecret() string {
	return getEnvironmentValue("GOOGLE_CLIENT_SECRET")
}
