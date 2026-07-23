package config

// AllowedRawConfigFiles whitelists which files the generic raw config
// editor may touch — both the local html UI's own /config pages and
// internal/cloudagent's relayed rawconfig.* actions share this one
// list, since a file name from either surface ultimately becomes a
// filesystem path (see Store.RawFile/SaveRaw) and the two HTTP-facing
// layers must never disagree about what's editable.
var AllowedRawConfigFiles = []string{
	RptConfFile,
	IaxConfFile,
	UsbradioConfFile,
	SimpleusbConfFile,
	"voter.conf",
	ExtensionsConfFile,
}

// IsAllowedRawConfigFile reports whether name is one of
// AllowedRawConfigFiles.
func IsAllowedRawConfigFile(name string) bool {
	for _, f := range AllowedRawConfigFiles {
		if f == name {
			return true
		}
	}
	return false
}
