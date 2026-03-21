package queueinspector

import (
	"os"

	"github.com/anish749/oncetask/oncetask"
)

// getEnv returns the task environment from the ONCE_TASK_ENV environment variable.
func getEnv() string {
	return os.Getenv(oncetask.EnvVariable)
}
