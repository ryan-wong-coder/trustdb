package sdfsigner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	EnvAdapterPath       = "TRUSTDB_SDF_ADAPTER"
	EnvAdapterConfigFile = "TRUSTDB_SDF_ADAPTER_CONFIG_FILE"
	EnvDeviceRef         = "TRUSTDB_SDF_DEVICE_REF"
	EnvCredentialRef     = "TRUSTDB_SDF_CREDENTIAL_REF"
	EnvCredentialFile    = "TRUSTDB_SDF_CREDENTIAL_FILE"
	EnvCapabilities      = "TRUSTDB_SDF_CAPABILITIES"
	EnvKEKID             = "TRUSTDB_SDF_KEK_ID"
	EnvKEKIndex          = "TRUSTDB_SDF_KEK_INDEX"
	EnvPluginID          = "TRUSTDB_SDF_PLUGIN_ID"
	EnvMaxConcurrency    = "TRUSTDB_SDF_MAX_CONCURRENCY"
)

type Environment struct {
	AdapterPath   string
	AdapterConfig []byte
	Config        Config
}

// LoadEnvironment reads only explicitly named SDF variables. Credentials are
// never accepted inline or through command arguments.
func LoadEnvironment() (Environment, error) {
	adapterPath := strings.TrimSpace(os.Getenv(EnvAdapterPath))
	if !validAbsolutePath(adapterPath) {
		return Environment{}, envError(EnvAdapterPath, "must be an absolute bounded adapter-library path")
	}
	configPath := strings.TrimSpace(os.Getenv(EnvAdapterConfigFile))
	if !validAbsolutePath(configPath) {
		return Environment{}, envError(EnvAdapterConfigFile, "must be an absolute bounded path")
	}
	adapterConfig, err := readBoundedRegularFile(context.Background(), configPath, MaxAdapterConfigBytes)
	if err != nil {
		return Environment{}, envError(EnvAdapterConfigFile, "cannot be read as an owner-only regular file")
	}
	fail := func(result Environment, configErr error) (Environment, error) {
		clear(adapterConfig)
		return result, configErr
	}
	deviceRef := strings.TrimSpace(os.Getenv(EnvDeviceRef))
	if !validIdentifier(deviceRef, 4096) {
		return fail(Environment{}, envError(EnvDeviceRef, "is required and malformed"))
	}
	credentialRef := strings.TrimSpace(os.Getenv(EnvCredentialRef))
	if !validIdentifier(credentialRef, 4096) {
		return fail(Environment{}, envError(EnvCredentialRef, "is required and malformed"))
	}
	credentialPath := strings.TrimSpace(os.Getenv(EnvCredentialFile))
	if !validAbsolutePath(credentialPath) {
		return fail(Environment{}, envError(EnvCredentialFile, "must be an absolute bounded path"))
	}
	credential, err := NewFileCredentialSource(credentialPath)
	if err != nil {
		return fail(Environment{}, err)
	}
	requiredCapabilities, err := parseCapabilities(strings.TrimSpace(os.Getenv(EnvCapabilities)))
	if err != nil {
		return fail(Environment{}, err)
	}
	kekID := strings.TrimSpace(os.Getenv(EnvKEKID))
	var kekIndex uint64
	if requiredCapabilities&SM4Capabilities != 0 {
		if !validIdentifier(kekID, 256) {
			return fail(Environment{}, envError(EnvKEKID, "is required for sm4-kek and malformed"))
		}
		kekIndex, err = strconv.ParseUint(strings.TrimSpace(os.Getenv(EnvKEKIndex)), 10, 32)
		if err != nil || kekIndex == 0 {
			return fail(Environment{}, envError(EnvKEKIndex, "must be a non-zero decimal integer for sm4-kek"))
		}
	} else if kekID != "" || strings.TrimSpace(os.Getenv(EnvKEKIndex)) != "" {
		return fail(Environment{}, envError(EnvCapabilities, "must include sm4-kek when a KEK is configured"))
	}
	pluginID := strings.TrimSpace(os.Getenv(EnvPluginID))
	if pluginID == "" {
		pluginID = DefaultPluginID
	}
	maxConcurrency := uint64(DefaultMaxConcurrentSigns)
	if raw := strings.TrimSpace(os.Getenv(EnvMaxConcurrency)); raw != "" {
		maxConcurrency, err = strconv.ParseUint(raw, 10, 32)
		if err != nil || maxConcurrency == 0 || maxConcurrency > 1024 {
			return fail(Environment{}, envError(EnvMaxConcurrency, "must be an integer between 1 and 1024"))
		}
	}
	return Environment{
		AdapterPath:   adapterPath,
		AdapterConfig: adapterConfig,
		Config: Config{
			PluginID:             pluginID,
			DeviceRef:            deviceRef,
			CredentialRef:        credentialRef,
			Credential:           credential,
			KEKID:                kekID,
			KEKIndex:             uint32(kekIndex),
			RequiredCapabilities: requiredCapabilities,
			MaxConcurrentSigns:   uint32(maxConcurrency),
		},
	}, nil
}

func parseCapabilities(raw string) (Capability, error) {
	if raw == "" {
		return SigningCapabilities, nil
	}
	var capabilities Capability
	seen := make(map[string]struct{}, 3)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if _, exists := seen[value]; exists {
			return 0, envError(EnvCapabilities, "contains a duplicate capability")
		}
		seen[value] = struct{}{}
		switch value {
		case "sign":
			capabilities |= SigningCapabilities
		case "random":
			capabilities |= CapabilityRandom
		case "sm4-kek":
			capabilities |= SM4Capabilities
		default:
			return 0, envError(EnvCapabilities, "contains an unsupported capability")
		}
	}
	if capabilities&SigningCapabilities != SigningCapabilities {
		return 0, envError(EnvCapabilities, "must include sign")
	}
	return capabilities, nil
}

func envError(name, message string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidConfiguration, name, message)
}

func validAbsolutePath(value string) bool {
	if value == "" || len(value) > 4096 || !filepath.IsAbs(value) || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return filepath.Clean(value) == value
}

func readBoundedRegularFile(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		!credentialFilePermissionsSafe(before) {
		return nil, newFault(faultPermission)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, newFault(faultPermission)
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, newFault(faultPermission)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(content) == 0 || int64(len(content)) > limit {
		clear(content)
		return nil, newFault(faultInvalid)
	}
	if err := ctx.Err(); err != nil {
		clear(content)
		return nil, err
	}
	return content, nil
}
