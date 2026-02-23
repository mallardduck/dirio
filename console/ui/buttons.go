package ui

import (
	"fmt"

	"github.com/a-h/templ"

	"github.com/mallardduck/dirio/consoleapi"
)

func ServiceAccountUpdateSecretClick(sa *consoleapi.ServiceAccount) templ.ComponentScript {
	return templ.ComponentScript{
		Name: "update-service-account-secret",
		Call: fmt.Sprintf(
			"const secret = prompt('Enter new secret key for %s:'); if(secret) { htmx.ajax('POST', '%s', {values: {secretKey: secret}, target: '#sa-section', swap: 'outerHTML'}) }",
			sa.AccessKey,
			PageURL("/service-accounts/"+sa.UUID+"/secret"),
		),
	}
}

func UserUpdateSecretClick(u *consoleapi.User) templ.ComponentScript {
	return templ.ComponentScript{
		Name: "update-user-secret",
		Call: fmt.Sprintf(
			"const secret = prompt('Enter new secret key for %s:'); if(secret) { htmx.ajax('POST', '%s', {values: {secretKey: secret}, target: '#users-section', swap: 'outerHTML'}) }",
			u.AccessKey,
			PageURL("/users/"+u.UUID+"/secret"),
		),
	}
}
