package app

import "context"

// PruneSystemLog runs the daily System log prune now, for tests.
func (a *App) PruneSystemLog(ctx context.Context) error { return a.pruneSystemLog(ctx) }

// TemplateTextKeys lists every catalog key the templates ask for by name, for tests.
func TemplateTextKeys() ([]string, error) { return templateTextKeys() }
