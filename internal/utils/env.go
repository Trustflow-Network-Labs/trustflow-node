package utils

import (
	"os"

	"github.com/joho/godotenv"
)

type EnvManager struct {
}

func NewEnvManager(path string) *EnvManager {
	return &EnvManager{}
}

// Load the .env file and returns value of provided key
func (em *EnvManager) EnvVariable(key string) string {
	err := godotenv.Load(".env")

	if err != nil {
		panic(err)
	}

	return os.Getenv(key)
}
