package promotion

import (
	"github.com/isometry/gh-promotion-app/internal/helpers"
	"log/slog"
)

type Context struct {
	Logger     *slog.Logger
	EventType  *string
	Owner      *string
	Repository *string
	BaseRef    *string
	HeadRef    *string
	HeadSHA    *string
	Promoter   *Promoter
}

func (p *Context) LogValue() slog.Value {
	logAttr := make([]slog.Attr, 1, 6)
	logAttr[0] = slog.String("eventType", helpers.String(p.EventType))
	if p.Owner != nil {
		logAttr = append(logAttr, slog.String("owner", helpers.String(p.Owner)))
	}
	if p.Repository != nil {
		logAttr = append(logAttr, slog.String("repository", helpers.String(p.Repository)))
	}
	if p.HeadSHA != nil {
		logAttr = append(logAttr, slog.String("headSHA", helpers.String(p.HeadSHA)))
	}
	if p.HeadRef != nil {
		logAttr = append(logAttr, slog.String("headRef", helpers.String(p.HeadRef)))
	}
	if p.BaseRef != nil {
		logAttr = append(logAttr, slog.String("baseRef", helpers.String(p.BaseRef)))
	}
	return slog.GroupValue(logAttr...)
}
