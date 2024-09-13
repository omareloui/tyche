package config

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

func GetDefaultCurrency() string {
	return "EGP"
}
