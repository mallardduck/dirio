package s3

import "github.com/mallardduck/dirio/internal/logging"

var s3Logger = logging.Default().With("component", "s3")
