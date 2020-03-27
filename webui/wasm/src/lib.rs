use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use wasm_bindgen::prelude::*;
use wasm_bindgen::JsCast;

extern crate console_error_panic_hook;
use std::panic;

#[derive(Serialize, Deserialize)]
pub struct Record {
    Name: String,
    Type: i32,
    Value: String,
}

#[derive(Serialize, Deserialize)]
pub struct DbData {
    Tablename: String,
    Records: Vec<Vec<Record>>,
    ColNames: Vec<String>,
    RowCount: i32,
    ColCount: i32,
    SortCol: String,
    SortDir: String,
    Offset: i32,
}

const GOLDEN_RATIO_CONJUGATE: f64 = 0.6180;
const DEBUG: bool = false;

// * Helper functions, as the web_sys pieces don't seem capable of being stored in globals *
fn window() -> web_sys::Window {
    web_sys::window().expect("no global `window` exists")
}

fn document() -> web_sys::Document {
    window()
        .document()
        .expect("should have a document on window")
}

// draw_bar_chart draws a simple bar chart, with a colour palette generated from the provided seed value
#[wasm_bindgen]
pub fn draw_bar_chart(palette: f64, js_data: &JsValue) {
    // Show better panic messages on the javascript console.  Useful for development
    panic::set_hook(Box::new(console_error_panic_hook::hook));

    // * Import the data from the web page *
    let data: DbData = js_data.into_serde().unwrap();
    let rows = data.Records;

    // TODO: Sort the categories so the draw order of barcharts is stable

    // TODO: Sorting the categories in some more useful way can come later

    // Count the number of items for each category
    let mut highest_val = 0;
    let mut item_counts: HashMap<&String, u32> = HashMap::new();
    for row in &rows {
        let cat_name = &row[10].Value;
        let item_count = &row[12].Value;
        let item_count: u32 = item_count.parse().unwrap();
        if item_counts.contains_key(&cat_name) {
            let c = item_counts[cat_name];
            item_counts.insert(cat_name, c + item_count);
        } else {
            item_counts.insert(cat_name, item_count);
        }
    }

    // Display the number of items for each category to the javascript console, for debugging purposes
    if DEBUG {
        for (cat, cnt) in &item_counts {
            web_sys::console::log_4(
                &"Category: ".into(),
                &(*cat).into(),
                &" Count: ".into(),
                &(*cnt).into(),
            );
        }
    }

    // Determine the highest count value, so we can automatically size the graph to fit
    for (_cat, cnt) in &item_counts {
        if cnt > &highest_val {
            highest_val = *cnt;
        }
    }
    if DEBUG {
        web_sys::console::log_2(&"Highest count: ".into(), &highest_val.into());
    }

    // * Canvas setup *

    let canvas: web_sys::HtmlCanvasElement = document()
        .get_element_by_id("barchart")
        .unwrap()
        .dyn_into::<web_sys::HtmlCanvasElement>()
        .unwrap();
    let mut width = canvas.width() as f64;
    let mut height = canvas.height() as f64;

    // Handle window resizing
    let current_body_width = window().inner_width().unwrap().as_f64().unwrap();
    let current_body_height = window().inner_height().unwrap().as_f64().unwrap();
    if current_body_width != width || current_body_height != height {
        width = current_body_width;
        height = current_body_height;
        canvas.set_attribute("width", &width.to_string());
        canvas.set_attribute("height", &height.to_string());
    }
    // canvas.set_tab_index(0); // Not sure if this is needed

    // Get the 2D context for the canvas
    let ctx = canvas
        .get_context("2d")
        .unwrap()
        .unwrap()
        .dyn_into::<web_sys::CanvasRenderingContext2d>()
        .unwrap();

    // * Bar graph setup *

    // Calculate the values used for controlling the graph positioning and display
    let axis_caption_font_size = 20.0;
    let axis_thickness = 5.0;
    let border = 2.0;
    let gap = 2.0;
    let graph_border = 50.0;
    let text_gap = 5.0;
    let title_font_size = 25.0;
    let unit_size = 3.0;
    let x_count_font_size = 18.0;
    let x_label_font_size = 20.0;
    let top = border + gap;
    let display_width = width - border - 1.0;
    let display_height = height - border - 1.0;
    let vert_size = highest_val as f64 * unit_size;
    let base_line = display_height - ((display_height - vert_size) / 2.0);
    let bar_label_y = base_line + x_label_font_size + text_gap + axis_thickness + text_gap;
    let y_base = base_line + axis_thickness + text_gap;
    let y_top = base_line - (vert_size * 1.2);
    let y_length = y_base - y_top;

    // TODO: Calculate the graph height based upon the available size of the canvas, instead of using the current fixed unit size

    // TODO: Calculate the font sizes based upon the whether they fit in their general space
    //       We should be able to get the font size scaling down decently, without a huge effort

    // Calculate the bar size, gap, and centering based upon the number of bars
    let num_bars = item_counts.len() as f64;
    let horiz_size = display_width - (graph_border * 2.0);
    let b = horiz_size / num_bars;
    let bar_width = b * 0.6;
    let bar_gap = b - bar_width;
    let mut bar_left = ((graph_border * 2.0) + bar_gap) / 2.0;
    let axis_left = ((graph_border * 2.0) + bar_gap) / 2.0;
    let axis_right = axis_left
        + (num_bars * bar_width)
        + ((num_bars - 1.0) * bar_gap)
        + axis_thickness
        + text_gap;

    // Calculate the y axis units of measurement
    let (y_max, y_step) = axis_max(highest_val);
    let y_unit = y_length / y_max;
    let y_unit_step = y_unit * y_step;

    // Clear the background
    ctx.set_fill_style(&"white".into());
    ctx.fill_rect(0.0, 0.0, width, height);

    // Draw y axis marker lines
    let y_marker_font_size = 12.0;
    let y_marker_left = axis_left - axis_thickness - text_gap - 5.0;
    ctx.set_stroke_style(&"rgb(220, 220, 220)".into());
    ctx.set_fill_style(&"black".into());
    ctx.set_font(&format!("{}", y_marker_font_size));
    ctx.set_text_align(&"right");
    let mut i = y_base;
    while i >= y_top {
        let marker_label = &format!("{}", (y_base - i) / y_unit);
        let marker_metrics = ctx.measure_text(&marker_label).unwrap();
        let y_marker_width = marker_metrics.width();
        ctx.begin_path();
        ctx.move_to(y_marker_left - y_marker_width, i);
        ctx.line_to(axis_right, i);
        ctx.stroke();
        ctx.fill_text(marker_label, axis_left - 15.0, i - 4.0);
        i -= y_unit_step;
    }

    // Draw simple bar graph using the category data
    let mut hue = palette;
    ctx.set_stroke_style(&"black".into());
    ctx.set_text_align(&"center");
    let mut font_size;
    for (label, num) in &item_counts {
        // Draw the bar
        let bar_height = *num as f64 * unit_size;
        hue += GOLDEN_RATIO_CONJUGATE;
        hue = hue % 1.0;
        ctx.set_fill_style(&hsv_to_rgb(hue, 0.5, 0.95).into());
        ctx.begin_path();
        ctx.move_to(bar_left, base_line);
        ctx.line_to(bar_left + bar_width, base_line);
        ctx.line_to(bar_left + bar_width, base_line - bar_height);
        ctx.line_to(bar_left, base_line - bar_height);
        ctx.close_path();
        ctx.fill();
        ctx.stroke();
        ctx.set_fill_style(&"black".into());

        // Draw the bar label horizontally centered
        font_size = format!("{}px serif", x_label_font_size);
        ctx.set_font(&font_size);
        let text_left = bar_width / 2.0;
        ctx.fill_text(label, bar_left + text_left, bar_label_y);

        // Draw the item count centered above the top of the bar
        ctx.set_font(&format!("{}px serif", x_count_font_size));
        ctx.fill_text(
            &format!("{}", num),
            bar_left + text_left,
            base_line - bar_height - text_gap,
        );
        bar_left += bar_gap + bar_width;
    }

    // Draw axis
    ctx.set_line_width(axis_thickness);
    ctx.begin_path();
    ctx.move_to(axis_right, y_base);
    ctx.line_to(axis_left - axis_thickness - text_gap, y_base);
    ctx.line_to(axis_left - axis_thickness - text_gap, y_top);
    ctx.stroke();

    // Draw title
    let title = "Marine Litter Survey - Keep Northern Ireland Beautiful";
    ctx.set_font(&format!("bold {}px serif", title_font_size));
    ctx.set_text_align(&"center");
    let title_left = display_width / 2.0;
    ctx.fill_text(title, title_left, top + title_font_size + 20.0);

    // Draw Y axis caption
    // Info on how to rotate text on the canvas:
    //   https://newspaint.wordpress.com/2014/05/22/writing-rotated-text-on-a-javascript-canvas/
    let spin_x = display_width / 2.0;
    let spin_y = y_top + ((y_base - y_top) / 2.0) + 50.0; // TODO: Figure out why 50 works well here, then autocalculate it for other graphs
    let y_axis_caption = "Number of items";
    ctx.save();
    ctx.translate(spin_x, spin_y);
    ctx.rotate(3.0 * std::f64::consts::PI / 2.0);
    ctx.set_font(&format!("italic {}px serif", axis_caption_font_size));
    ctx.set_fill_style(&"black".into());
    ctx.set_text_align(&"left");
    ctx.fill_text(
        y_axis_caption,
        0.0,
        -spin_x + axis_left - text_gap - axis_caption_font_size - 30.0, // TODO: Figure out why 30 works well here, then autocalculate it for other graphs
    );
    ctx.restore();

    // Draw X axis caption
    let x_axis_caption = "Category";
    ctx.set_font(&format!("italic {}px serif", axis_caption_font_size));
    let cap_left = display_width / 2.0;
    ctx.fill_text(
        x_axis_caption,
        cap_left,
        bar_label_y + text_gap + axis_caption_font_size,
    );

    // Draw a border around the graph area
    ctx.set_line_width(2.0);
    ctx.set_stroke_style(&"white".into());
    ctx.begin_path();
    ctx.move_to(0.0, 0.0);
    ctx.line_to(width, 0.0);
    ctx.line_to(width, height);
    ctx.line_to(0.0, height);
    ctx.close_path();
    ctx.stroke();
    ctx.set_line_width(2.0);
    ctx.set_stroke_style(&"black".into());
    ctx.begin_path();
    ctx.move_to(border, border);
    ctx.line_to(display_width, border);
    ctx.line_to(display_width, display_height);
    ctx.line_to(border, display_height);
    ctx.close_path();
    ctx.stroke();
}

// Ported from the JS here: https://martin.ankerl.com/2009/12/09/how-to-create-random-colors-programmatically/
fn hsv_to_rgb(h: f64, s: f64, v: f64) -> String {
    let hi = h * 6.0;
    let f = h * 6.0 - hi;
    let p = v * (1.0 - s);
    let q = v * (1.0 - f * s);
    let t = v * (1.0 - (1.0 - f) * s);

    let hi = hi as i32;
    let mut r: f64 = 0.0;
    let mut g: f64 = 0.0;
    let mut b: f64 = 0.0;
    if hi == 0 {
        r = v;
        g = t;
        b = p;
    }
    if hi == 1 {
        r = q;
        g = v;
        b = p;
    }
    if hi == 2 {
        r = p;
        g = v;
        b = t;
    }
    if hi == 3 {
        r = p;
        g = q;
        b = v;
    }
    if hi == 4 {
        r = t;
        g = p;
        b = v;
    }
    if hi == 5 {
        r = v;
        g = p;
        b = q;
    }

    let red = (r * 256.0) as i32;
    let green = (g * 256.0) as i32;
    let blue = (b * 256.0) as i32;
    return format!("rgb({}, {}, {})", red, green, blue);
}

// axis_max calculates the maximum value for a given axis, and the step value to use when drawing its grid lines
fn axis_max(val: u32) -> (f64, f64) {
    let val = val as f64;
    if val < 10.0 {
        return (10.0, 1.0);
    }

    // If val is less than 100, return val rounded up to the next 10
    if val < 100.0 {
        let x = val % 10.0;
        return (val + 10.0 - x, 10.0);
    }

    // If val is less than 500, return val rounded up to the next 50
    if val < 500.0 {
        let x = val % 50.0;
        return (val + 50.0 - x, 50.0);
    }
    (1000.0, 100.0)
}
