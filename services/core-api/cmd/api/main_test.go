package main

import "testing"

func TestDevQuickSignInRequiresTheDevelopmentEnvironmentAndFlag(t *testing.T) {
	tests := []struct {
		name   string
		appEnv string
		flag   string
		want   bool
	}{
		{name: "local development flag", appEnv: "development", flag: "true", want: true},
		{name: "default environment is development", flag: "on", want: true},
		{name: "production disables the flag", appEnv: "production", flag: "true", want: false},
		{name: "flag remains opt-in", appEnv: "development", flag: "false", want: false},
		{name: "unknown environment disables the flag", appEnv: "staging", flag: "true", want: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("APP_ENV", testCase.appEnv)
			t.Setenv("DEV_QUICK_SIGN_IN_ENABLED", testCase.flag)
			if got := devQuickSignInEnabled(); got != testCase.want {
				t.Fatalf("devQuickSignInEnabled() = %v, want %v", got, testCase.want)
			}
		})
	}
}
