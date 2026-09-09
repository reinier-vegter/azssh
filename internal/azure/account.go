package azure

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Account identifies the active Azure CLI account for cache isolation.
type Account struct {
	Cloud     string
	TenantID  string
	Principal string
}

// ActiveAccount reads local Azure CLI context. Resource inventory is still
// obtained only through the Azure SDK and Azure Resource Graph.
func ActiveAccount() (Account, error) {
	output, err := exec.Command("az", "account", "show", "--output", "json").Output()
	if err != nil {
		return Account{}, fmt.Errorf("read Azure CLI account context: %w", err)
	}
	var response struct {
		EnvironmentName string `json:"environmentName"`
		TenantID        string `json:"tenantId"`
		User            struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return Account{}, fmt.Errorf("decode Azure CLI account context: %w", err)
	}
	account := Account{
		Cloud:     strings.TrimSpace(response.EnvironmentName),
		TenantID:  strings.TrimSpace(response.TenantID),
		Principal: strings.TrimSpace(response.User.Name),
	}
	if account.Cloud == "" || account.TenantID == "" || account.Principal == "" {
		return Account{}, fmt.Errorf("Azure CLI account context is incomplete")
	}
	return account, nil
}
