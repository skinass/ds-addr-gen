package main

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"log"
	"syscall/js"

	"github.com/creasty/defaults"
	_ "github.com/shurcooL/go-goon"
	"github.com/signintech/gopdf"
	"github.com/skip2/go-qrcode"
	yaml "gopkg.in/yaml.v3"
)

const defaultConfig = `#описание секций со стеллажами. 
#для них генерятся адреса для каждой полки на каждом стеллаже.
sections:
  - zone: "A"
    shelfs: 10
    rows: 6
  - zone: "O"
    shelfs: 12
    rows: 4

#настройки рендеринга.
render:
  rows: 6 #сколько строк стикеров на одной странице пдф
  columns: 2 #сколько колонок стикеров на одной странице пдф
  font_size: 60 #размер текста
  qrcode_size: 60 #размер qr-кода
  qrcode_resolution: 256 #качество qr-кода
  orientation: vertical #настройка листов. портретная(вертикальное расположение) или альбомная(горизонтальное)
  # orientation: horizontal
  sticker_left_offset: 10 #насколько стикер делает отступ слева 
  space_between_qr_and_text: 20 #расстояние между текстом и qr-кодом
`

type GenConf struct {
	Sections []struct {
		Zone   string `yaml:"zone"`
		Shelfs int    `yaml:"shelfs"`
		Rows   int    `yaml:"rows"`
	} `yaml:"sections"`
	Render struct {
		Rows                  int    `yaml:"rows"`
		Columns               int    `yaml:"columns"`
		FontSize              int    `yaml:"font_size" default:"60"`
		QRCodeSize            int    `yaml:"qrcode_size" default:"60"`
		QRCodeResolution      int    `yaml:"qrcode_resolution" default:"256"`
		Orientation           string `yaml:"orientation" default:"vertical"`
		StickerLeftOffset     int    `yaml:"sticker_left_offset" default:"10"`
		SpaceBetweenQRAndText int    `yaml:"space_between_qr_and_text" default:"20"`
	}
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

func GenAddrList(conf *GenConf) []Addr {
	res := []Addr{}
	for _, section := range conf.Sections {
		for shelfN := 1; shelfN <= section.Shelfs; shelfN++ {
			for rowN := 1; rowN <= section.Rows; rowN++ {
				text := fmt.Sprintf("%s%02d-%d", section.Zone, shelfN, rowN)
				res = append(res, Addr{
					QRCodeData: text,
					Text:       text,
				})
			}
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

	stickerVerticalSize := H / float64(conf.Render.Rows)

	addrsQueue := addrs
	cnt := 0
FillStickersOnPages:
	for len(addrsQueue) > 0 {
		pdf.AddPage()
		for i := float64(0); i < W; i += W / float64(conf.Render.Columns) {
			for j := float64(0); j < H; j += H / float64(conf.Render.Rows) {

				if len(addrsQueue) == 0 {
					break FillStickersOnPages
				}
				currentAddr := addrsQueue[0]
				addrsQueue = addrsQueue[1:]

				fmt.Println(currentAddr, i, j)
				cnt++
				AddOneSticker(&pdf, conf, i, j, stickerVerticalSize, currentAddr)
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
	addrList := GenAddrList(conf)
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
