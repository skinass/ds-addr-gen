package main

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"syscall/js"

	"github.com/creasty/defaults"
	brace "github.com/kujtimiihoxha/go-brace-expansion"
	_ "github.com/shurcooL/go-goon"
	"github.com/signintech/gopdf"
	"github.com/skip2/go-qrcode"
	yaml "gopkg.in/yaml.v3"
)

// #описание секций со стеллажами.
// #для них генерятся адреса для каждой полки на каждом стеллаже.
// sections:
//   - zone: "A"
//     shelfs: 2
//     rows: 3
//   - zone: "O"
//     shelfs: 1
//     rows: 4

const defaultConfig = `
#описание при помощи brace патернов
#полезно чтоб сгенерить конкретные недостающие этикетки 
addrs:
- Z{01..04}•{1..7} #сгенерит Z01•1,Z01•3,Z02•1,Z02•3
- Z12•4 #сгенерит Z12•4




#настройки рендеринга.
render:
  rows: 9 #сколько строк стикеров на одной странице пдф
  columns: 3 #сколько колонок стикеров на одной странице пдф
  font_size: 40 #размер текста
  qrcode_size: 50 #размер qr-кода
  qrcode_resolution: 512 #качество qr-кода
  orientation: vertical #настройка листов. портретная(вертикальное расположение) или альбомная(горизонтальное)
  # orientation: horizontal
  sticker_left_offset: 15 #насколько стикер делает отступ слева 
  space_between_qr_and_text: 15 #расстояние между текстом и qr-кодом
  top_bot_offsets: 5
  left_right_offsets: 5
  add_stroke: true
`

type GenConf struct {
	Sections GenConfSections `yaml:"sections"`
	Addrs    []string        `yaml:"addrs"`
	Render   GenConfRender   `yaml:"render"`
}

type GenConfSections []struct {
	Zone   string `yaml:"zone"`
	Shelfs int    `yaml:"shelfs"`
	Rows   int    `yaml:"rows"`
}
type GenConfRender struct {
	Rows                  int    `yaml:"rows"`
	Columns               int    `yaml:"columns"`
	FontSize              int    `yaml:"font_size" default:"60"`
	QRCodeSize            int    `yaml:"qrcode_size" default:"60"`
	QRCodeResolution      int    `yaml:"qrcode_resolution" default:"256"`
	Orientation           string `yaml:"orientation" default:"vertical"`
	StickerLeftOffset     int    `yaml:"sticker_left_offset" default:"10"`
	SpaceBetweenQRAndText int    `yaml:"space_between_qr_and_text" default:"20"`
	TopBotOffsets         int    `yaml:"top_bot_offsets" default:"0"`
	LeftRightOffsets      int    `yaml:"left_right_offsets" default:"0"`
	AddStroke             bool   `yaml:"add_stroke" default:"true"`
}

func GetGenConf(confRaw []byte) *GenConf {
	conf := &GenConf{}
	defaults.Set(conf)

	err := yaml.Unmarshal(confRaw, conf)
	if err != nil {
		log.Println(err)
	}

	return conf
}

type Addr struct {
	QRCodeData string
	Text       string
}

var dotReplacer = strings.NewReplacer("-", "•")

func GenAddrListFromSections(sections GenConfSections) []Addr {
	res := []Addr{}
	for _, section := range sections {
		for shelfN := 1; shelfN <= section.Shelfs; shelfN++ {
			for rowN := 1; rowN <= section.Rows; rowN++ {
				text := fmt.Sprintf("%s%02d•%d", section.Zone, shelfN, rowN)
				text = dotReplacer.Replace(text)
				res = append(res, Addr{
					QRCodeData: text,
					Text:       text,
				})
			}
		}
	}
	return res
}

func GenAddrListFromPatterns(addrs []string) []Addr {
	res := []Addr{}
	for _, addrPattern := range addrs {
		for _, addr := range brace.Expand(addrPattern) {
			text := addr
			text = dotReplacer.Replace(text)
			res = append(res, Addr{
				QRCodeData: text,
				Text:       text,
			})
		}
	}
	return res
}

//go:embed arial/ARIAL.TTF
var ARIAL_TTF_DATA []byte

func CreatePdf(conf *GenConf, addrs []Addr) *gopdf.GoPdf {
	pdf := gopdf.GoPdf{}
	pdf.SetTransparency(gopdf.Transparency{Alpha: 0})

	var W, H float64 = 842, 595
	if conf.Render.Orientation == "horizontal" {
		pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4Landscape}) //842x595
		W, H = 842, 595
	} else {
		pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4}) //842x595
		W, H = 595, 842
	}

	err := pdf.AddTTFFontData("arial", ARIAL_TTF_DATA)
	if err != nil {
		log.Println(err)
	}
	err = pdf.SetFont("arial", "", conf.Render.FontSize)
	if err != nil {
		log.Println(err)
	}

	stickerVerticalSize := (H - 2*float64(conf.Render.TopBotOffsets)) / float64(conf.Render.Rows)

	addrsQueue := addrs
	cnt := 0
FillStickersOnPages:
	for len(addrsQueue) > 0 {
		pdf.AddPage()
		for j := float64(conf.Render.TopBotOffsets); j < H-2*float64(conf.Render.TopBotOffsets)-((stickerVerticalSize-float64(conf.Render.QRCodeSize))/2); j += (H - 2*float64(conf.Render.TopBotOffsets)) / float64(conf.Render.Rows) {
			for i := float64(conf.Render.LeftRightOffsets); i < W-2*float64(conf.Render.LeftRightOffsets); i += (W - 2*float64(conf.Render.LeftRightOffsets)) / float64(conf.Render.Columns) {
				if len(addrsQueue) == 0 {
					break FillStickersOnPages
				}
				currentAddr := addrsQueue[0]
				addrsQueue = addrsQueue[1:]

				fmt.Println(currentAddr, i, j)
				cnt++
				AddOneSticker(&pdf, conf, i, j, stickerVerticalSize, currentAddr)

				if conf.Render.AddStroke {
					pdf.SetTransparency(gopdf.Transparency{Alpha: 0.5, BlendModeType: gopdf.ColorBurn})
					pdf.SetLineType("dotted")
					pdf.SetStrokeColor(0, 0, 0)
					pdf.SetLineWidth(2)
					// pdf.Line(
					// 	i,
					// 	j,
					// 	i+(W-2*float64(conf.Render.LeftRightOffsets))/float64(conf.Render.Columns),
					// 	j,
					// )
					if cnt%conf.Render.Columns != 0 {
						pdf.Line(
							i+(W-2*float64(conf.Render.LeftRightOffsets))/float64(conf.Render.Columns),
							j,
							i+(W-2*float64(conf.Render.LeftRightOffsets))/float64(conf.Render.Columns),
							j+(H-2*float64(conf.Render.TopBotOffsets))/float64(conf.Render.Rows),
						)
					}
					if (cnt-1)%(conf.Render.Rows*conf.Render.Columns) < (conf.Render.Rows-1)*conf.Render.Columns {
						pdf.Line(
							i+(W-2*float64(conf.Render.LeftRightOffsets))/float64(conf.Render.Columns),
							j+(H-2*float64(conf.Render.TopBotOffsets))/float64(conf.Render.Rows),
							i,
							j+(H-2*float64(conf.Render.TopBotOffsets))/float64(conf.Render.Rows),
						)
					}
					// pdf.Line(
					// 	i,
					// 	j+(H-2*float64(conf.Render.TopBotOffsets))/float64(conf.Render.Rows),
					// 	i,
					// 	j,
					// )
					pdf.ClearTransparency()
				}
			}
		}
	}
	fmt.Println("total addresses added to pdf:", cnt)

	return &pdf
}

func AddOneSticker(pdf *gopdf.GoPdf, conf *GenConf, x, y float64, verticalSize float64, addr Addr) {
	var (
		topOffset  = (verticalSize - float64(conf.Render.QRCodeSize)) / 2
		leftOffset = float64(conf.Render.StickerLeftOffset)
	)

	q, err := qrcode.New(addr.QRCodeData, qrcode.Medium)
	if err != nil {
		log.Println(err)
	}
	q.DisableBorder = true
	png, err := q.PNG(conf.Render.QRCodeSize)
	if err != nil {
		log.Println(err)
	}

	img, err := gopdf.ImageHolderByBytes(png)
	if err != nil {
		log.Println(err)
	}

	pdf.ImageByHolder(img, x+leftOffset, y+topOffset,
		&gopdf.Rect{W: float64(conf.Render.QRCodeSize), H: float64(conf.Render.QRCodeSize)},
	)

	pdf.SetXY(
		x+leftOffset+float64(conf.Render.QRCodeSize)+float64(conf.Render.SpaceBetweenQRAndText),
		y+topOffset+(float64(conf.Render.QRCodeSize)-float64(conf.Render.FontSize)/1.33)/2)
	pdf.Cell(nil, addr.Text)
}

func updatePdfData() {
	//hide download button
	doc := js.Global().Get("document")
	pdfHTML := doc.Call("getElementById", "pdf_download")
	pdfHTML.Get("style").Set("display", "none")

	//take config from textarea
	configHTML := doc.Call("getElementById", "generate_config")
	confStr := configHTML.Get("value").String()

	//generate pdf
	conf := GetGenConf([]byte(confStr))
	addrList := append(
		GenAddrListFromPatterns(conf.Addrs),
		GenAddrListFromSections(conf.Sections)...)
	pdf := CreatePdf(conf, addrList)

	//update pdf on download link
	pdfBase64Str := base64.StdEncoding.EncodeToString(pdf.GetBytesPdf())
	pdfHTML.Set("href", fmt.Sprintf("data:application/pdf;name=ds-addr-gen.pdf;base64,%s", pdfBase64Str))
	pdfHTML.Set("download", fmt.Sprintf("ds-addr-gen.pdf"))
	pdfHTML.Get("style").Set("display", "inline-block")

	previewHTML1 := doc.Call("getElementById", "preview1")
	previewHTML1.Set("src", fmt.Sprintf("data:application/pdf;base64,%s", pdfBase64Str))
	// previewHTML2 := doc.Call("getElementById", "preview2")
	// previewHTML2.Set("src", fmt.Sprintf("data:application/pdf;base64,%s", pdfBase64Str))
	// previewHTML := doc.Call("getElementById", "preview1")
	// previewHTML.Set("data", fmt.Sprintf("data:application/pdf;base64,%s", pdfBase64Str))
}

func main() {
	//add default config into textarea
	doc := js.Global().Get("document")
	configHTML := doc.Call("getElementById", "generate_config")
	configHTML.Set("value", defaultConfig)

	//register pdf updater on button click
	generateButtonHTML := doc.Call("getElementById", "generate_button")
	generateButtonHTML.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		updatePdfData()
		return nil
	}))

	//keep golang runtime alive
	select {}
}

// local run
// GOOS=js GOARCH=wasm go build -o ./static/ds-addr-gen.wasm main.go && goexec 'http.ListenAndServe(`:8080`, http.FileServer(http.Dir(`./static`)))'
