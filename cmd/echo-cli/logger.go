package main

import "echo-cli/internal/logger"

// log 复用全局 logger，附加 CLI 组件标识。
var log = logger.Named("cli")
