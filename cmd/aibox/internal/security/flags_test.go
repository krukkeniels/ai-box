package security

import (
	"strings"
	"testing"
)

func TestDefaultFlags(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")

	if f.CapDrop != "ALL" {
		t.Errorf("CapDrop = %q, want %q", f.CapDrop, "ALL")
	}
	if !f.NoNewPrivileges {
		t.Error("NoNewPrivileges should be true")
	}
	if !f.ReadOnly {
		t.Error("ReadOnly should be true")
	}
	if f.User != "1000:1000" {
		t.Errorf("User = %q, want %q", f.User, "1000:1000")
	}
	if f.SeccompProfile != "/etc/aibox/seccomp.json" {
		t.Errorf("SeccompProfile = %q, want %q", f.SeccompProfile, "/etc/aibox/seccomp.json")
	}
	if f.AppArmorProfile != "aibox-sandbox" {
		t.Errorf("AppArmorProfile = %q, want %q", f.AppArmorProfile, "aibox-sandbox")
	}
}

func TestBuildArgs(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")
	args := f.BuildArgs()

	required := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--user=1000:1000",
		"--security-opt=seccomp=/etc/aibox/seccomp.json",
		"--security-opt=apparmor=aibox-sandbox",
	}

	for _, want := range required {
		found := false
		for _, arg := range args {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BuildArgs() missing %q", want)
		}
	}
}

func TestBuildArgs_NoAppArmor(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")
	f.AppArmorProfile = ""
	args := f.BuildArgs()

	for _, arg := range args {
		if strings.Contains(arg, "apparmor") {
			t.Errorf("BuildArgs() should not include apparmor flag when profile is empty, got %q", arg)
		}
	}
}

func TestValidate_ValidFlags(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")
	if err := f.Validate(); err != nil {
		t.Errorf("Validate() returned error for valid flags: %v", err)
	}
}

func TestValidate_InvalidCapDrop(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")
	f.CapDrop = "NET_ADMIN"
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for non-ALL CapDrop")
	}
	if !strings.Contains(err.Error(), "cap-drop must be ALL") {
		t.Errorf("error should mention cap-drop, got: %v", err)
	}
}

func TestValidate_NoNewPrivilegesDisabled(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")
	f.NoNewPrivileges = false
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() should return error when NoNewPrivileges is false")
	}
	if !strings.Contains(err.Error(), "no-new-privileges") {
		t.Errorf("error should mention no-new-privileges, got: %v", err)
	}
}

func TestValidate_ReadOnlyDisabled(t *testing.T) {
	f := DefaultFlags("/etc/aibox/seccomp.json")
	f.ReadOnly = false
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() should return error when ReadOnly is false")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error should mention read-only, got: %v", err)
	}
}

func TestValidate_RootUser(t *testing.T) {
	rootUsers := []string{"", "0", "0:0", "root", "root:root"}
	for _, u := range rootUsers {
		f := DefaultFlags("/etc/aibox/seccomp.json")
		f.User = u
		err := f.Validate()
		if err == nil {
			t.Errorf("Validate() should return error for root user %q", u)
		}
		if err != nil && !strings.Contains(err.Error(), "non-root") {
			t.Errorf("error should mention non-root for user %q, got: %v", u, err)
		}
	}
}

func TestValidate_EmptySeccomp(t *testing.T) {
	f := DefaultFlags("")
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for empty seccomp profile")
	}
	if !strings.Contains(err.Error(), "seccomp profile") {
		t.Errorf("error should mention seccomp profile, got: %v", err)
	}
}

func TestValidate_MultipleViolations(t *testing.T) {
	f := SecurityFlags{} // all zero values
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for zero-value flags")
	}
	errStr := err.Error()
	// Should report all violations at once.
	if !strings.Contains(errStr, "cap-drop") {
		t.Error("missing cap-drop violation")
	}
	if !strings.Contains(errStr, "no-new-privileges") {
		t.Error("missing no-new-privileges violation")
	}
	if !strings.Contains(errStr, "read-only") {
		t.Error("missing read-only violation")
	}
	if !strings.Contains(errStr, "non-root") {
		t.Error("missing non-root violation")
	}
	if !strings.Contains(errStr, "seccomp") {
		t.Error("missing seccomp violation")
	}
}

func TestValidateArgs_AllPresent(t *testing.T) {
	args := []string{
		"run", "-d",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--user=1000:1000",
		"--security-opt=seccomp=/etc/aibox/seccomp.json",
		"--security-opt=apparmor=aibox-sandbox",
		"harbor.internal/aibox/base:24.04",
	}
	if err := ValidateArgs(args); err != nil {
		t.Errorf("ValidateArgs() returned error for valid args: %v", err)
	}
}

func TestValidateArgs_MissingCapDrop(t *testing.T) {
	args := []string{
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--user=1000:1000",
		"--security-opt=seccomp=/etc/aibox/seccomp.json",
	}
	err := ValidateArgs(args)
	if err == nil {
		t.Fatal("ValidateArgs() should return error when --cap-drop=ALL is missing")
	}
	if !strings.Contains(err.Error(), "--cap-drop=ALL") {
		t.Errorf("error should mention --cap-drop=ALL, got: %v", err)
	}
}

func TestValidateArgs_RootUserRejected(t *testing.T) {
	rootArgs := []string{"--user=0", "--user=0:0", "--user=root", "--user=root:root"}
	for _, rootArg := range rootArgs {
		args := []string{
			"--cap-drop=ALL",
			"--security-opt=no-new-privileges:true",
			"--read-only",
			rootArg,
			"--security-opt=seccomp=/etc/aibox/seccomp.json",
		}
		err := ValidateArgs(args)
		if err == nil {
			t.Errorf("ValidateArgs() should reject root user arg %q", rootArg)
		}
	}
}

func TestValidateArgs_UnconfinedSeccompRejected(t *testing.T) {
	args := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--user=1000:1000",
		"--security-opt=seccomp=unconfined",
	}
	err := ValidateArgs(args)
	if err == nil {
		t.Fatal("ValidateArgs() should reject seccomp=unconfined")
	}
}

func TestValidateArgs_EmptySlice(t *testing.T) {
	err := ValidateArgs(nil)
	if err == nil {
		t.Fatal("ValidateArgs() should return error for empty args")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "refusing to launch") {
		t.Errorf("error should say 'refusing to launch', got: %v", err)
	}
}

// validBaseArgs returns a minimal set of args that satisfy all base security checks.
func validBaseArgs() []string {
	return []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--user=1000:1000",
		"--security-opt=seccomp=/etc/aibox/seccomp.json",
	}
}

func TestValidateArgsWithExpectations_GVisorExpected(t *testing.T) {
	// Base args without --runtime=runsc should fail when gVisor is expected.
	args := validBaseArgs()
	err := ValidateArgsWithExpectations(args, true, false)
	if err == nil {
		t.Fatal("should fail when gVisor is expected but --runtime=runsc is missing")
	}
	if !strings.Contains(err.Error(), "--runtime=runsc") {
		t.Errorf("error should mention --runtime=runsc, got: %v", err)
	}
}

func TestValidateArgsWithExpectations_AppArmorExpected(t *testing.T) {
	// Base args without apparmor should fail when AppArmor is expected.
	args := validBaseArgs()
	err := ValidateArgsWithExpectations(args, false, true)
	if err == nil {
		t.Fatal("should fail when AppArmor is expected but apparmor flag is missing")
	}
	if !strings.Contains(err.Error(), "apparmor") {
		t.Errorf("error should mention apparmor, got: %v", err)
	}
}

func TestValidateArgsWithExpectations_AllPresent(t *testing.T) {
	args := append(validBaseArgs(),
		"--runtime=runsc",
		"--security-opt=apparmor=aibox-sandbox",
	)
	if err := ValidateArgsWithExpectations(args, true, true); err != nil {
		t.Errorf("should pass when all expected flags are present: %v", err)
	}
}

func TestValidateArgsWithExpectations_BothExpectedBothMissing(t *testing.T) {
	args := validBaseArgs()
	err := ValidateArgsWithExpectations(args, true, true)
	if err == nil {
		t.Fatal("should fail when both gVisor and AppArmor are expected but missing")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "--runtime=runsc") {
		t.Error("error should mention --runtime=runsc")
	}
	if !strings.Contains(errStr, "apparmor") {
		t.Error("error should mention apparmor")
	}
}

func TestValidateArgsWithExpectations_NeitherExpected(t *testing.T) {
	// When neither is expected, base args alone should pass.
	args := validBaseArgs()
	if err := ValidateArgsWithExpectations(args, false, false); err != nil {
		t.Errorf("should pass when neither gVisor nor AppArmor is expected: %v", err)
	}
}
