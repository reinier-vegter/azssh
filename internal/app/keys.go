package app

import "charm.land/bubbles/v2/key"

type keyMap struct {
	FilterSubscriptions key.Binding
	Favorite            key.Binding
	Help                key.Binding
	Refresh             key.Binding
	Route               key.Binding
	Transfer            key.Binding
	Mount               key.Binding
	Quit                key.Binding
	Apply               key.Binding
	Cancel              key.Binding
	Toggle              key.Binding
	All                 key.Binding
	None                key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		FilterSubscriptions: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filters")),
		Favorite:            key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "favorite")),
		Help:                key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Refresh:             key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Route:               key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "route")),
		Transfer:            key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transfer")),
		Mount:               key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mount")),
		Quit:                key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Apply:               key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
		Cancel:              key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Toggle:              key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
		All:                 key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all")),
		None:                key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "none")),
	}
}
