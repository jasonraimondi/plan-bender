package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectInstallMethod(t *testing.T) {
	tests := []struct {
		name string
		path string
		want InstallMethod
	}{
		{
			name: "npm global install",
			path: "/usr/local/lib/node_modules/@jasonraimondi/plan-bender-darwin-arm64/bin/plan-bender",
			want: InstallMethodNPM,
		},
		{
			name: "npm local install",
			path: "/home/user/project/node_modules/@jasonraimondi/plan-bender-linux-x64/bin/plan-bender",
			want: InstallMethodNPM,
		},
		{
			name: "npm nested node_modules",
			path: "/home/user/project/node_modules/.pnpm/@jasonraimondi+plan-bender@1.0.0/node_modules/@jasonraimondi/plan-bender-linux-x64/bin/plan-bender",
			want: InstallMethodNPM,
		},
		{
			name: "direct binary in usr local bin",
			path: "/usr/local/bin/plan-bender",
			want: InstallMethodDirect,
		},
		{
			name: "direct binary in home bin",
			path: "/home/user/.local/bin/plan-bender",
			want: InstallMethodDirect,
		},
		{
			name: "direct binary in go bin",
			path: "/home/user/go/bin/plan-bender",
			want: InstallMethodDirect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectInstallMethod(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}
