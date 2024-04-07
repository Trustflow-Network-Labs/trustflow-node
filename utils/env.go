package utils

import (
	"os"

	"github.com/joho/godotenv"
)

// Load the .env file and returns value of provided key
func EnvVariable(key string) string {
	err := godotenv.Load(".env")

	if err != nil {
		panic(err)
	}

	return os.Getenv(key)
}
