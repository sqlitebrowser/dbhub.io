package main

import (
	"strconv"
	"syscall/js"
)

const (
	// 4 digits should be plenty for our purposes
	goldenRatioConjugate = 0.6180
	Pi                   = 3.1415
)

func main() {}

// DrawBarChart draws a simple bar chart, with a colour palette generated from the provided seed value
//go:export DrawBarChart
func DrawBarChart(palette float32) {
	doc := js.Global().Get("document")
	canvasEl := doc.Call("getElementById", "barchart")
	width := canvasEl.Get("clientWidth").Int()
	height := canvasEl.Get("clientHeight").Int()
	canvasEl.Call("setAttribute", "width", width)
	canvasEl.Call("setAttribute", "height", height)
	canvasEl.Set("tabIndex", 0) // Not sure if this is needed
	ctx := canvasEl.Call("getContext", "2d")

	// Retrieve the data
	db := js.Global().Get("barChartData")
	rows := db.Get("Records")
	numRows := rows.Length()

	// Count the number of items for each category
	highestVal := 0
	itemCounts := make(map[string]int)
	var row js.Value
	for i, n := 0, numRows; i < n; i++ {
		row = rows.Index(i)
		catName := row.Index(10).Get("Value").String()
		itemCount, err := strconv.Atoi(row.Index(12).Get("Value").String())
		if err != nil {
			println(err)
		}
		c := itemCounts[catName]
		itemCounts[catName] = c + itemCount
	}

	// Determine the highest count value, so we can automatically size the graph to fit
	for _, itemCount := range itemCounts {
		if itemCount > highestVal {
			highestVal = itemCount
		}
	}

	// Calculate the values used for controlling the graph positioning and display
	axisCaptionFontSize := 20
	axisThickness := 5
	border := 2
	gap := 2
	graphBorder := 50
	textGap := 5
	titleFontSize := 25
	unitSize := 3
	xCountFontSize := 18
	xLabelFontSize := 20
	top := border + gap
	displayWidth := width - border - 1
	displayHeight := height - border - 1
	vertSize := highestVal * unitSize
	baseLine := displayHeight - ((displayHeight - vertSize) / 2)
	barLabelY := baseLine + xLabelFontSize + textGap + axisThickness + textGap
	yBase := baseLine + axisThickness + textGap
	yTop := baseLine - int(float64(vertSize)*1.2)
	yLength := yBase - yTop

	// TODO: Calculate the graph height based upon the available size of the canvas, instead of using the current fixed unit size

	// TODO: Calculate the font sizes based upon the whether they fit in their general space
	//       We should be able to get the font size scaling down decently, without a huge effort

	// Calculate the bar size, gap, and centering based upon the number of bars
	numBars := len(itemCounts)
	horizSize := displayWidth - (graphBorder * 2)
	b := float64(horizSize) / float64(numBars)
	barWidth := int(b * 0.6)
	barGap := int(b - float64(barWidth))
	barLeft := ((graphBorder * 2) + barGap) / 2
	axisLeft := ((graphBorder * 2) + barGap) / 2
	axisRight := axisLeft + (numBars * barWidth) + ((numBars - 1) * barGap) + axisThickness + textGap

	// Calculate the y axis units of measurement
	yMax, yStep := axisMax(highestVal)
	yUnit := yLength / yMax
	yUnitStep := yUnit * yStep

	// TODO: Sort the categories in some useful way

	// Clear the background
	ctx.Set("fillStyle", "white")
	ctx.Call("fillRect", 0, 0, width, height)

	// Draw y axis marker lines
	yMarkerFontSize := 12
	yMarkerLeft := axisLeft - axisThickness - textGap - 5
	ctx.Set("strokeStyle", "rgb(220, 220, 220)")
	ctx.Set("fillStyle", "black")
	ctx.Set("font", strconv.FormatInt(int64(yMarkerFontSize), 10)+"px serif")
	ctx.Set("textAlign", "right")
	for i := float64(yBase); i >= float64(yTop); i -= float64(yUnitStep) {
		markerLabel := strconv.FormatInt(int64((float64(yBase)-i)/float64(yUnit)), 10)
		markerMet := ctx.Call("measureText", markerLabel)
		yMarkerWidth := int(markerMet.Get("width").Float())
		ctx.Call("beginPath")
		ctx.Call("moveTo", yMarkerLeft-yMarkerWidth, i)
		ctx.Call("lineTo", axisRight, i)
		ctx.Call("stroke")
		ctx.Call("fillText", markerLabel, axisLeft-15, i-4)
	}

	// Draw simple bar graph using the category data
	hue := float64(palette)
	ctx.Set("strokeStyle", "black")
	ctx.Set("textAlign", "center")
	for label, num := range itemCounts {
		barHeight := num * unitSize
		hue += goldenRatioConjugate
		hue = hue - float64(int(hue)) // Simplified version of "hue % 1"
		ctx.Set("font", "bold "+strconv.FormatInt(int64(xLabelFontSize), 10)+"px serif")
		ctx.Set("fillStyle", hsvToRgb(hue, 0.5, 0.95))
		ctx.Call("beginPath")
		ctx.Call("moveTo", barLeft, baseLine)
		ctx.Call("lineTo", barLeft+barWidth, baseLine)
		ctx.Call("lineTo", barLeft+barWidth, baseLine-barHeight)
		ctx.Call("lineTo", barLeft, baseLine-barHeight)
		ctx.Call("closePath")
		ctx.Call("fill")
		ctx.Call("stroke")
		ctx.Set("fillStyle", "black")

		// Draw the bar label horizontally centered
		textLeft := float64(barWidth) / 2
		ctx.Call("fillText", label, barLeft+int(textLeft), barLabelY)

		// Draw the item count centered above the top of the bar
		ctx.Set("font", strconv.FormatInt(int64(xCountFontSize), 10)+"px serif")
		s := strconv.FormatInt(int64(num), 10)
		textLeft = float64(barWidth) / 2
		ctx.Call("fillText", s, barLeft+int(textLeft), baseLine-barHeight-textGap)
		barLeft += barGap + barWidth
	}

	// Draw axis
	ctx.Set("lineWidth", axisThickness)
	ctx.Call("beginPath")
	ctx.Call("moveTo", axisRight, yBase)
	ctx.Call("lineTo", axisLeft-axisThickness-textGap, yBase)
	ctx.Call("lineTo", axisLeft-axisThickness-textGap, yTop)
	ctx.Call("stroke")

	// Draw title
	title := "Marine Litter Survey - Keep Northern Ireland Beautiful"
	ctx.Set("font", "bold "+strconv.FormatInt(int64(titleFontSize), 10)+"px serif")
	ctx.Set("textAlign", "center")
	titleLeft := displayWidth / 2
	ctx.Call("fillText", title, titleLeft, top+titleFontSize+20)

	// Draw Y axis caption
	// Info on how to rotate text on the canvas:
	//   https://newspaint.wordpress.com/2014/05/22/writing-rotated-text-on-a-javascript-canvas/
	spinX := displayWidth / 2
	spinY := yTop + ((yBase - yTop) / 2) + 50 // TODO: Figure out why 50 works well here, then autocalculate it for other graphs
	yAxisCaption := "Number of items"
	ctx.Call("save")
	ctx.Call("translate", spinX, spinY)
	ctx.Call("rotate", 3*Pi/2)
	ctx.Set("font", "italic "+strconv.FormatInt(int64(axisCaptionFontSize), 10)+"px serif")
	ctx.Set("fillStyle", "black")
	ctx.Set("textAlign", "left")
	ctx.Call("fillText", yAxisCaption, 0, -spinX+axisLeft-textGap-int(axisCaptionFontSize)-30) // TODO: Figure out why 30 works well here, then autocalculate it for other graphs
	ctx.Call("restore")

	// Draw X axis caption
	xAxisCaption := "Category"
	ctx.Set("font", "italic "+strconv.FormatInt(int64(axisCaptionFontSize), 10)+"px serif")
	capLeft := displayWidth / 2
	ctx.Call("fillText", xAxisCaption, capLeft, barLabelY+textGap+axisCaptionFontSize)

	// Draw a border around the graph area
	ctx.Set("lineWidth", "2")
	ctx.Set("strokeStyle", "white")
	ctx.Call("beginPath")
	ctx.Call("moveTo", 0, 0)
	ctx.Call("lineTo", width, 0)
	ctx.Call("lineTo", width, height)
	ctx.Call("lineTo", 0, height)
	ctx.Call("closePath")
	ctx.Call("stroke")
	ctx.Set("lineWidth", "2")
	ctx.Set("strokeStyle", "black")
	ctx.Call("beginPath")
	ctx.Call("moveTo", border, border)
	ctx.Call("lineTo", displayWidth, border)
	ctx.Call("lineTo", displayWidth, displayHeight)
	ctx.Call("lineTo", border, displayHeight)
	ctx.Call("closePath")
	ctx.Call("stroke")
}

// Ported from the JS here: https://martin.ankerl.com/2009/12/09/how-to-create-random-colors-programmatically/
func hsvToRgb(h, s, v float64) string {
	hi := h * 6
	f := h*6 - hi
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)

	hiInt := int(hi)
	var r, g, b float64
	if hiInt == 0 {
		r, g, b = v, t, p
	}
	if hiInt == 1 {
		r, g, b = q, v, p
	}
	if hiInt == 2 {
		r, g, b = p, v, t
	}
	if hiInt == 3 {
		r, g, b = p, q, v
	}
	if hiInt == 4 {
		r, g, b = t, p, v
	}
	if hiInt == 5 {
		r, g, b = v, p, q
	}

	red := int(r * 256)
	green := int(g * 256)
	blue := int(b * 256)
	return "rgb(" + strconv.FormatInt(int64(red), 10) + ", " + strconv.FormatInt(int64(green), 10) + ", " + strconv.FormatInt(int64(blue), 10) + ")"
}

// axisMax calculates the maximum value for a given axis, and the step value to use when drawing its grid lines
func axisMax(val int) (int, int) {
	if val < 10 {
		return 10, 1
	}

	// If val is less than 100, return val rounded up to the next 10
	if val < 100 {
		x := val % 10
		return val + 10 - x, 10
	}

	// If val is less than 500, return val rounded up to the next 50
	if val < 500 {
		x := val % 50
		return val + 50 - x, 50
	}
	return 1000, 100
}
