//go:build windows

package main

import (
	"image"
	"image/color"

	"github.com/lxn/walk"
)

func belarusFlagImage() image.Image {
	const w, h = 54, 36
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	red := color.RGBA{R: 0xCF, G: 0x31, B: 0x35, A: 255}
	stripe := h / 3
	for y := 0; y < h; y++ {
		c := white
		if y >= stripe && y < 2*stripe {
			c = red
		}
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func belarusFlagBitmap() (*walk.Bitmap, error) {
	return walk.NewBitmapFromImage(belarusFlagImage())
}

func addBelarusHeader(parent walk.Container) error {
	row, err := walk.NewComposite(parent)
	if err != nil {
		return err
	}
	rowLayout := walk.NewHBoxLayout()
	rowLayout.SetSpacing(10)
	if err := row.SetLayout(rowLayout); err != nil {
		return err
	}

	flagView, err := walk.NewImageView(row)
	if err != nil {
		return err
	}
	bmp, err := belarusFlagBitmap()
	if err != nil {
		return err
	}
	if err := flagView.SetImage(bmp); err != nil {
		return err
	}
	flagView.SetMode(walk.ImageViewModeShrink)
	if err := flagView.SetMinMaxSize(walk.Size{Width: 54, Height: 36}, walk.Size{Width: 54, Height: 36}); err != nil {
		return err
	}

	titleLabel, err := walk.NewLabel(row)
	if err != nil {
		return err
	}
	if err := titleLabel.SetText("BELARUS CHAMP CLICKER"); err != nil {
		return err
	}
	font, err := walk.NewFont("Segoe UI", 11, walk.FontBold)
	if err != nil {
		return err
	}
	titleLabel.SetFont(font)

	return nil
}
