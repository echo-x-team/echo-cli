package execution

import "echo-cli/internal/logger"

// log 复用全局 logger。
var log = logger.Named("engine")
