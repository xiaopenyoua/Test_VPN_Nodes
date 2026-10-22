//go:build !darwin && !windows

package netx

// DetectSystem 在不支持自动探测的平台返回空值，由上层回落到直连语义。
func DetectSystem() (string, string) { return "", "" }
