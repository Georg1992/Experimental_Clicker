//go:build windows

package main

import (
	"github.com/lxn/walk"
)

type clickerStatus int

const (
	clickerStatusStopped clickerStatus = iota
	clickerStatusRunning
)

var statusColors = map[clickerStatus]walk.Color{
	clickerStatusStopped: walk.RGB(220, 53, 53),
	clickerStatusRunning: walk.RGB(46, 184, 70),
}

var statusTexts = map[clickerStatus]string{
	clickerStatusStopped: "OFF",
	clickerStatusRunning: "ON",
}

const statusBadgeWidth = 110
const statusBadgeHeight = 40

type statusBadge struct {
	*walk.CustomWidget
	status clickerStatus
	font   *walk.Font
}

func newStatusBadge(parent walk.Container) (*statusBadge, error) {
	font, err := walk.NewFont("Segoe UI", 14, walk.FontBold)
	if err != nil {
		return nil, err
	}

	badge := &statusBadge{
		status: clickerStatusStopped,
		font:   font,
	}
	cw, err := walk.NewCustomWidgetPixels(parent, 0, badge.paint)
	if err != nil {
		font.Dispose()
		return nil, err
	}
	cw.SetPaintMode(walk.PaintBuffered)
	badge.CustomWidget = cw
	if err := cw.SetMinMaxSize(
		walk.Size{Width: statusBadgeWidth, Height: statusBadgeHeight},
		walk.Size{Width: statusBadgeWidth, Height: statusBadgeHeight},
	); err != nil {
		font.Dispose()
		return nil, err
	}
	return badge, nil
}

func (b *statusBadge) paint(canvas *walk.Canvas, bounds walk.Rectangle) error {
	color := statusColors[b.status]
	brush, err := walk.NewSolidColorBrush(color)
	if err != nil {
		return err
	}
	defer brush.Dispose()

	if err := canvas.FillRectanglePixels(brush, bounds); err != nil {
		return err
	}

	return canvas.DrawTextPixels(
		statusTexts[b.status],
		b.font,
		walk.RGB(255, 255, 255),
		bounds,
		walk.TextCenter|walk.TextVCenter,
	)
}

func (b *statusBadge) SetStatus(status clickerStatus) {
	if b.status == status {
		return
	}
	b.status = status
	b.Invalidate()
}
