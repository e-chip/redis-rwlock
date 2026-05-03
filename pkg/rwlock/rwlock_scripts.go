package rwlock

import _ "embed"

//go:embed lua/read-lock.lua
var readLockScript string

//go:embed lua/read-unlock.lua
var readUnlockScript string

//go:embed lua/lock-refresh.lua
var lockRefreshScript string

//go:embed lua/write-lock.lua
var writeLockScript string

//go:embed lua/write-unlock.lua
var writeUnlockScript string
