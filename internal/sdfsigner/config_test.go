package sdfsigner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadEnvironmentRequiresExplicitSecretFiles(t *testing.T) {
	clearSDFEnvironment(t)
	temp := t.TempDir()
	adapter := filepath.Join(temp, "libtrustdb-sdf-adapter.so")
	config := filepath.Join(temp, "adapter.conf")
	credential := filepath.Join(temp, "credential")
	for path, content := range map[string]string{
		config:     "vendor_driver=/opt/vendor/libsdf.so\n",
		credential: "846295\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(EnvAdapterPath, adapter)
	t.Setenv(EnvAdapterConfigFile, config)
	t.Setenv(EnvDeviceRef, "sdf-production")
	t.Setenv(EnvCredentialRef, "receipt-operator")
	t.Setenv(EnvCredentialFile, credential)
	t.Setenv(EnvCapabilities, "sign,random,sm4-kek")
	t.Setenv(EnvKEKID, "backup-kek-v1")
	t.Setenv(EnvKEKIndex, "11")
	if runtime.GOOS == "windows" {
		if _, err := LoadEnvironment(); err == nil {
			t.Fatal("Windows accepted files without qualified owner-only DACL validation")
		}
		return
	}
	environment, err := LoadEnvironment()
	if err != nil {
		t.Fatalf("LoadEnvironment() error = %v", err)
	}
	defer clear(environment.AdapterConfig)
	if environment.AdapterPath != adapter ||
		string(environment.AdapterConfig) != "vendor_driver=/opt/vendor/libsdf.so\n" ||
		environment.Config.DeviceRef != "sdf-production" ||
		environment.Config.CredentialRef != "receipt-operator" ||
		environment.Config.RequiredCapabilities != AllCapabilities ||
		environment.Config.KEKID != "backup-kek-v1" ||
		environment.Config.KEKIndex != 11 ||
		environment.Config.PluginID != DefaultPluginID ||
		environment.Config.MaxConcurrentSigns != DefaultMaxConcurrentSigns {
		t.Fatalf("environment = %+v", environment.Config)
	}
}

func TestLoadEnvironmentRejectsInlineCredential(t *testing.T) {
	clearSDFEnvironment(t)
	t.Setenv(EnvAdapterPath, "/opt/trustdb/libtrustdb-sdf-adapter.so")
	t.Setenv(EnvAdapterConfigFile, "/etc/trustdb/sdf-adapter.conf")
	t.Setenv(EnvDeviceRef, "sdf-production")
	t.Setenv(EnvCredentialRef, "receipt-operator")
	t.Setenv(EnvKEKID, "backup-kek-v1")
	t.Setenv(EnvKEKIndex, "11")
	t.Setenv("TRUSTDB_SDF_CREDENTIAL", "must-not-be-supported")
	if _, err := LoadEnvironment(); err == nil {
		t.Fatal("LoadEnvironment() accepted an inline credential")
	}
}

func TestLoadEnvironmentRedactsConfigurationPaths(t *testing.T) {
	clearSDFEnvironment(t)
	secretPath := filepath.Join(t.TempDir(), "customer-device-secret")
	t.Setenv(EnvAdapterPath, "/relative/../bad")
	t.Setenv(EnvAdapterConfigFile, secretPath)
	_, err := LoadEnvironment()
	if err == nil {
		t.Fatal("LoadEnvironment() accepted an unsafe adapter path")
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(err.Error(), "customer-device-secret") {
		t.Fatalf("configuration error leaked path: %v", err)
	}
}

func clearSDFEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		EnvAdapterPath,
		EnvAdapterConfigFile,
		EnvDeviceRef,
		EnvCredentialRef,
		EnvCredentialFile,
		EnvCapabilities,
		EnvKEKID,
		EnvKEKIndex,
		EnvPluginID,
		EnvMaxConcurrency,
		"TRUSTDB_SDF_CREDENTIAL",
	} {
		t.Setenv(name, "")
	}
}
