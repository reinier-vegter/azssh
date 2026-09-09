package cache

// Preferences contains user choices that are independent of Azure inventory.
type Preferences struct {
	HiddenSubscriptionIDs []string `json:"hiddenSubscriptionIDs"`
}

type preferencesEnvelope struct {
	SchemaVersion int         `json:"schemaVersion"`
	Data          Preferences `json:"data"`
}

// LoadPreferences reads the stored user preferences.
func (s *Store) LoadPreferences() (Preferences, error) {
	var envelope preferencesEnvelope
	if err := s.load("preferences.json", &envelope); err != nil {
		return Preferences{}, err
	}
	if envelope.SchemaVersion != schemaVersion {
		return Preferences{}, ErrUnsupportedSchema
	}
	return envelope.Data, nil
}

// SavePreferences atomically saves user preferences.
func (s *Store) SavePreferences(preferences Preferences) error {
	return s.save("preferences.json", preferencesEnvelope{SchemaVersion: schemaVersion, Data: preferences})
}
