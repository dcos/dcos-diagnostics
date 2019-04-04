package api

// UnitPropertiesResponse is a structure to unmarshal dbus.GetunitProperties response
type UnitPropertiesResponse struct {
	ID             string `mapstructure:"Id"`
	LoadState      string
	ActiveState    string
	SubState       string
	Description    string
	ExecMainStatus int

	InactiveExitTimestampMonotonic  uint64
	ActiveEnterTimestampMonotonic   uint64
	ActiveExitTimestampMonotonic    uint64
	InactiveEnterTimestampMonotonic uint64
}
