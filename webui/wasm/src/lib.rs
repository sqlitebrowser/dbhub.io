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
    Title: String,
    Tablename: String,
    XAxisLabel: String,
    YAxisLabel: String,
    Records: Vec<Vec<Record>>,
    ColNames: Vec<String>,
    RowCount: i32,
    ColCount: i32,
}

struct DrawObject {
    name: String,
    num: u32,
}

impl DrawObject {
    pub fn new(name: String, num: u32) -> Self {
        DrawObject {
            name,
            num
        }
    }
}

enum OrderBy {
    CategoryName = 0,
    CategoryValue = 1,
}

enum OrderDirection {
    OrderDescending = 0,
    OrderAscending = 1,
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
pub fn draw_bar_chart(palette: f64, js_data: &JsValue, order_by: u32, order_direction: u32) {
    // Show better panic messages on the javascript console.  Useful for development
    panic::set_hook(Box::new(console_error_panic_hook::hook));

    // * Import the data from the web page *
    let data: DbData = js_data.into_serde().unwrap();
    let rows = data.Records;

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

    // * Sort the category data, so the draw order of bars doesn't change when the browser window is resized *

    let mut draw_order: Vec<DrawObject> = vec![];
    for (label, num) in &item_counts {
        draw_order.push(DrawObject::new(label.to_string(), num.clone()));
    }

    // Sort by the users chosen sort order
    if order_by == OrderBy::CategoryName as u32 {
        // Sort by category name
        draw_order.sort_by(|a, b| b.name.cmp(&a.name));
    } else {
        // Sort by item total
        draw_order.sort_by(|a, b| b.num.cmp(&a.num));
    }

    // Reverse the sort order if desired
    if order_direction == OrderDirection::OrderDescending as u32 {
        draw_order.reverse();
    }

    // * Canvas setup *

    let canvas: web_sys::HtmlCanvasElement = document()
        .get_element_by_id("barchart")
        .unwrap()
        .dyn_into::<web_sys::HtmlCanvasElement>()
        .unwrap();
    let mut canvas_width = canvas.width() as f64;
    let mut canvas_height = canvas.height() as f64;

    if DEBUG {
        web_sys::console::log_1(&format!("Canvas element width: {} Canvas element height: {}", &canvas_width, &canvas_height).into());
    }

    // Handle window resizing
    let current_body_width = window().inner_width().unwrap().as_f64().unwrap();
    let current_body_height = window().inner_height().unwrap().as_f64().unwrap();
    if current_body_width != canvas_width || current_body_height != canvas_height {
        canvas_width = current_body_width;
        canvas_height = current_body_height;
        canvas.set_attribute("width", &canvas_width.to_string());
        canvas.set_attribute("height", &canvas_height.to_string());
    }

    if DEBUG {
        web_sys::console::log_1(&format!("Canvas current body width: {} Canvas current body canvas_height: {}", &canvas_width, &canvas_height).into());
    }

    // Get the 2D context for the canvas
    let ctx = canvas
        .get_context("2d")
        .unwrap()
        .unwrap()
        .dyn_into::<web_sys::CanvasRenderingContext2d>()
        .unwrap();

    // * Bar graph setup *

    // Fixed value pieces
    let border = 2.0;
    let gap = 2.0;
    let graph_border = 50.0;

    // Calculate the values used for controlling the graph positioning and display
    let area_root = (canvas_height * canvas_width).sqrt();
    let y_axis_caption_font_height = area_root * 0.015;
    let x_axis_caption_font_height = area_root * 0.015;
    let text_gap = area_root * 0.006;
    let title_font_height = area_root * 0.025;
    let title_font_spacing = area_root * 0.025;
    let x_count_font_height = area_root * 0.015;
    let x_label_font_height = area_root * 0.015;
    let y_axis_marker_font_height = area_root * 0.015;
    let axis_thickness = area_root * 0.004;

    let top = border + gap;
    let display_width = canvas_width - border - 1.0;
    let display_height = canvas_height - border - 1.0;

    // FIXME: The area_root piece here is a placeholder, and should instead probably be the height of the X axis info below the X axis line
    let vert_size = canvas_height - (2.0 * border) - (2.0 * gap) - (title_font_height + title_font_height) - (area_root * 0.2);
    let bar_height_unit_size = vert_size / highest_val as f64;

    // FIXME: The area_root piece here is a placeholder, and should instead probably be the height of the X axis info below the X axis line
    let base_line = display_height - ((display_height - vert_size) / 2.0) + (area_root * 0.05);
    let bar_label_y = base_line + x_label_font_height + text_gap + axis_thickness + text_gap;
    let y_base = base_line + axis_thickness + text_gap;
    let y_top = base_line - (vert_size * 1.2);
    let y_length = y_base - y_top;

    if DEBUG {
        web_sys::console::log_1(&format!("area_root: {}", &area_root).into());
        web_sys::console::log_1(&format!("y_axis_caption_font_height: {}", &y_axis_caption_font_height).into());
        web_sys::console::log_1(&format!("x_axis_caption_font_height: {}", &x_axis_caption_font_height).into());
        web_sys::console::log_1(&format!("axis_thickness: {}", &axis_thickness).into());
        web_sys::console::log_1(&format!("border: {}", &border).into());
        web_sys::console::log_1(&format!("gap: {}", &gap).into());
        web_sys::console::log_1(&format!("graph_border: {}", &graph_border).into());
        web_sys::console::log_1(&format!("text_gap: {}", &text_gap).into());
        web_sys::console::log_1(&format!("title_font_height: {}", &title_font_height).into());
        web_sys::console::log_1(&format!("bar_height_unit_size: {}", &bar_height_unit_size).into());
        web_sys::console::log_1(&format!("x_count_font_height: {}", &x_count_font_height).into());
        web_sys::console::log_1(&format!("x_label_font_height: {}", &x_label_font_height).into());
        web_sys::console::log_1(&format!("top: {}", &top).into());
        web_sys::console::log_1(&format!("display_width: {}", &display_width).into());
        web_sys::console::log_1(&format!("display_height: {}", &display_height).into());
        web_sys::console::log_1(&format!("vert_size: {}", &vert_size).into());
        web_sys::console::log_1(&format!("base_line: {}", &base_line).into());
        web_sys::console::log_1(&format!("bar_label_y: {}", &bar_label_y).into());
        web_sys::console::log_1(&format!("y_base: {}", &y_base).into());
        web_sys::console::log_1(&format!("y_top: {}", &y_top).into());
        web_sys::console::log_1(&format!("y_length: {}", &y_length).into());
    }

    // TODO: Calculate the font sizes based upon the whether they fit in their general space

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
    let (y_axis_max_value, y_axis_step) = axis_max(highest_val);
    let y_unit = y_length / y_axis_max_value;
    let y_unit_step = y_unit * y_axis_step;

    if DEBUG {
        web_sys::console::log_1(&format!("Y axis max: {}, step {}, unit {}, unit step {}", &y_axis_max_value, &y_axis_step, &y_unit, &y_unit_step).into());
    }

    // Clear the background
    ctx.set_fill_style(&"white".into());
    ctx.fill_rect(0.0, 0.0, canvas_width, canvas_height);

    // Draw y axis marker lines
    let y_marker_left = axis_left - axis_thickness - text_gap - 5.0;
    ctx.set_stroke_style(&"rgb(220, 220, 220)".into());
    ctx.set_fill_style(&"black".into());
    ctx.set_font(&format!("{}pt serif", y_axis_marker_font_height));
    ctx.set_text_align(&"right");
    let mut y_axis_marker_largest_width = 0.0;
    let mut i = y_base;
    while i >= y_top {
        let marker_label = &format!("{} ", ((y_base - i) / y_unit).round());
        let marker_metrics = ctx.measure_text(&marker_label).unwrap();
        let y_axis_marker_width = marker_metrics.width().round();
        if y_axis_marker_width > y_axis_marker_largest_width {
            y_axis_marker_largest_width = y_axis_marker_width;
        }
        ctx.begin_path();
        ctx.move_to(y_marker_left - y_axis_marker_width, i);
        ctx.line_to(axis_right, i);
        ctx.stroke();
        ctx.fill_text(marker_label, axis_left - 15.0, i - 4.0);
        i -= y_unit_step;

        if DEBUG {
            web_sys::console::log_1(&format!(
                "Y axis marker '{}', width: {} height {}", &marker_label, &y_axis_marker_width, &y_axis_marker_font_height).into()
            );
        }
    }

    // Draw simple bar graph using the category data
    let mut hue = palette;
    ctx.set_stroke_style(&"black".into());
    ctx.set_text_align(&"center");
    for bar in &draw_order {
        // Draw the bar
        let label = &bar.name;
        let num = bar.num;
        let bar_height = num as f64 * bar_height_unit_size;
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
        ctx.set_font(&format!("{}pt serif", x_label_font_height));
        let text_left = bar_width / 2.0;
        ctx.fill_text(label, bar_left + text_left, bar_label_y);

        // Draw the item count centered above the top of the bar
        ctx.set_font(&format!("{}pt serif", x_count_font_height));
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
    let mut title = data.Title.as_str();
    if title.ends_with(".sqlite") {
        title = title.trim_end_matches(".sqlite")
    }
    if title.ends_with(".sqlite3") {
        title = title.trim_end_matches(".sqlite3")
    }
    if title.ends_with(".db") {
        title = title.trim_end_matches(".db")
    }
    ctx.set_font(&format!("bold {}pt serif", title_font_height));
    ctx.set_text_align(&"center");
    let title_left = display_width / 2.0;
    ctx.fill_text(title, title_left, top + title_font_height + title_font_spacing);

    // Draw Y axis caption
    // Info on how to rotate text on the canvas:
    //   https://newspaint.wordpress.com/2014/05/22/writing-rotated-text-on-a-javascript-canvas/
    let y_axis_caption = &data.YAxisLabel;
    let y_axis_caption_string = format!("italic {}pt serif", y_axis_caption_font_height);
    let y_axis_caption_metrics = ctx.measure_text(&y_axis_caption_string).unwrap();
    let y_axis_caption_width = y_axis_caption_metrics.width().round();
    let spin_x = display_width / 2.0;
    let spin_y = y_top + ((y_base - y_top) / 2.0);
    ctx.save();
    ctx.translate(spin_x, spin_y);
    ctx.rotate(3.0 * std::f64::consts::PI / 2.0);
    ctx.set_font(&y_axis_caption_string);
    ctx.set_fill_style(&"black".into());
    ctx.set_text_align(&"center");
    ctx.fill_text(
        y_axis_caption,
        0.0,
        -spin_x + axis_left - text_gap - y_axis_caption_font_height - y_axis_marker_largest_width,
    );
    ctx.restore();

    if DEBUG {
        web_sys::console::log_1(&format!("y_axis_caption_width: {}", &y_axis_caption_width).into());
    }

    // Draw X axis caption
    let x_axis_caption = &data.XAxisLabel;
    ctx.set_font(&format!("italic {}pt serif", x_axis_caption_font_height));
    let cap_left = display_width / 2.0;
    ctx.fill_text(
        x_axis_caption,
        cap_left,
        bar_label_y + text_gap + x_axis_caption_font_height,
    );

    // Draw a border around the graph area
    ctx.set_line_width(2.0);
    ctx.set_stroke_style(&"white".into());
    ctx.begin_path();
    ctx.move_to(0.0, 0.0);
    ctx.line_to(canvas_width, 0.0);
    ctx.line_to(canvas_width, canvas_height);
    ctx.line_to(0.0, canvas_height);
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
