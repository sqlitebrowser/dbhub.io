use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use wasm_bindgen::prelude::*;
use wasm_bindgen::JsCast;
use js_sys::Array;

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
    let area_border = 2.0;
    let display_width = canvas_width - border - 1.0;
    let display_height = canvas_height - border - 1.0;

    // Calculate the area available to each of the graph elements
    let graph_space_width = display_width * 0.9; // Graph area is allowed to use 90% of the canvas width.  The side borders get the remaining 10% (5% each)
    let graph_space_height = display_height * 0.8;

    let left_space_width = (display_width - graph_space_width) / 2.0;
    let left_space_height = display_height * 0.8;

    let right_space_width = left_space_width;
    let right_space_height = display_height * 0.8;

    let top_space_width = display_width;
    let top_space_height = (display_height - graph_space_height) / 2.0;

    let bottom_space_width = display_width;
    let bottom_space_height = top_space_height;

    // Derived co-ordinates
    let left_space_top = border + area_border + top_space_height;
    let left_space_left = border + area_border;
    let left_space_bottom = left_space_top + left_space_height;
    let left_space_right = left_space_left + left_space_width;

    let right_space_top = border + area_border + top_space_height;
    let right_space_left = border + area_border + left_space_width + graph_space_width - (area_border * 3.0);
    let right_space_bottom = right_space_top + right_space_height;
    let right_space_right = right_space_left + right_space_width;

    let top_space_top = border + area_border;
    let top_space_left = border + area_border;
    let top_space_bottom = top_space_top + top_space_height;
    let top_space_right = top_space_left + top_space_width - (area_border * 3.0);

    let bottom_space_top = border + area_border + top_space_height + graph_space_height;
    let bottom_space_left = border + area_border;
    let bottom_space_bottom = bottom_space_top + bottom_space_height - (area_border * 3.0);
    let bottom_space_right = bottom_space_left + bottom_space_width - (area_border * 3.0);

    let graph_space_left = left_space_right;
    let graph_space_top = top_space_bottom;
    let graph_space_bottom = bottom_space_top;
    let graph_space_right = right_space_left;

    // Clear the background
    ctx.set_fill_style(&"white".into());
    ctx.fill_rect(0.0, 0.0, canvas_width, canvas_height);

    if DEBUG {
        // * Draw borders around each area of space *

        // Left space
        ctx.set_line_width(area_border);
        ctx.set_stroke_style(&"blue".into());
        ctx.begin_path();
        ctx.move_to(left_space_left, left_space_top);
        ctx.line_to(left_space_right, left_space_top);
        ctx.line_to(left_space_right, left_space_bottom);
        ctx.line_to(left_space_left, left_space_bottom);
        ctx.close_path();
        ctx.stroke();

        // Right space
        ctx.set_stroke_style(&"green".into());
        ctx.begin_path();
        ctx.move_to(right_space_left, right_space_top);
        ctx.line_to(right_space_right, right_space_top);
        ctx.line_to(right_space_right, right_space_bottom);
        ctx.line_to(right_space_left, right_space_bottom);
        ctx.close_path();
        ctx.stroke();

        // Top space
        ctx.set_stroke_style(&"red".into());
        ctx.begin_path();
        ctx.move_to(top_space_left, top_space_top);
        ctx.line_to(top_space_right, top_space_top);
        ctx.line_to(top_space_right, top_space_bottom);
        ctx.line_to(top_space_left, top_space_bottom);
        ctx.close_path();
        ctx.stroke();

        // Bottom space
        ctx.set_stroke_style(&"cyan".into());
        ctx.begin_path();
        ctx.move_to(bottom_space_left, bottom_space_top);
        ctx.line_to(bottom_space_right, bottom_space_top);
        ctx.line_to(bottom_space_right, bottom_space_bottom);
        ctx.line_to(bottom_space_left, bottom_space_bottom);
        ctx.close_path();
        ctx.stroke();

        // Graph space
        ctx.set_stroke_style(&"magenta".into());
        ctx.begin_path();
        ctx.move_to(graph_space_left, graph_space_top);
        ctx.line_to(graph_space_right, graph_space_top);
        ctx.line_to(graph_space_right, graph_space_bottom);
        ctx.line_to(graph_space_left, graph_space_bottom);
        ctx.close_path();
        ctx.stroke();

        // * Draw center lines for each area of space *

        let dash = Array::new();
        dash.push(&"1".into());
        dash.push(&"3".into());
        ctx.save();
        ctx.set_line_width(1.0);
        ctx.set_stroke_style(&"grey".into());
        ctx.set_line_dash(&dash.into());

        // Left space
        ctx.begin_path();
        ctx.move_to(left_space_left + (left_space_width / 2.0), left_space_top);
        ctx.line_to(left_space_left + (left_space_width / 2.0), left_space_bottom);
        ctx.stroke();
        ctx.begin_path();
        ctx.move_to(left_space_left, left_space_top + (left_space_height / 2.0));
        ctx.line_to(left_space_right, left_space_top + (left_space_height / 2.0));
        ctx.stroke();

        // Right space
        ctx.begin_path();
        ctx.move_to(right_space_left + (right_space_width / 2.0), right_space_top);
        ctx.line_to(right_space_left + (right_space_width / 2.0), right_space_bottom);
        ctx.stroke();
        ctx.begin_path();
        ctx.move_to(right_space_left, right_space_top + (right_space_height / 2.0));
        ctx.line_to(right_space_right, right_space_top + (right_space_height / 2.0));
        ctx.stroke();

        // Top space
        ctx.begin_path();
        ctx.move_to(top_space_left + (top_space_width / 2.0), top_space_top);
        ctx.line_to(top_space_left + (top_space_width / 2.0), top_space_bottom);
        ctx.stroke();
        ctx.begin_path();
        ctx.move_to(top_space_left, top_space_top + (top_space_height / 2.0));
        ctx.line_to(top_space_right, top_space_top + (top_space_height / 2.0));
        ctx.stroke();

        // Bottom space
        ctx.begin_path();
        ctx.move_to(bottom_space_left + (bottom_space_width / 2.0), bottom_space_top);
        ctx.line_to(bottom_space_left + (bottom_space_width / 2.0), bottom_space_bottom);
        ctx.stroke();
        ctx.begin_path();
        ctx.move_to(bottom_space_left, bottom_space_top + (bottom_space_height / 2.0));
        ctx.line_to(bottom_space_right, bottom_space_top + (bottom_space_height / 2.0));
        ctx.stroke();

        ctx.restore();
    }

    // Calculate the values used for controlling the graph positioning and display
    // TODO: Add the table name in as a sub-heading
    let area_root = (canvas_height * canvas_width).sqrt(); // This seems like a ~simple + effective approach to handle scaling in either dimension
    let y_axis_caption_font_height = area_root * 0.015;
    let x_axis_caption_font_height = area_root * 0.015;
    let x_axis_caption_text_gap = area_root * 0.006;
    let title_font_height = area_root * 0.025;
    // let title_font_spacing = area_root * 0.025;
    let x_count_font_height = area_root * 0.015;
    let x_axis_label_font_height = area_root * 0.015;
    let y_axis_marker_font_height = area_root * 0.015;
    let axis_thickness = area_root * 0.004;

    let base_line = graph_space_bottom - axis_thickness - x_axis_label_font_height - (2.0 * x_axis_caption_text_gap);
    let vert_size = base_line - graph_space_top;
    let bar_height_unit_size = vert_size / highest_val as f64;
    let bar_label_y = graph_space_bottom;
    let bar_border = 1.0;
    let y_base = base_line + axis_thickness + x_axis_caption_text_gap;
    let y_top = graph_space_top;
    let y_length = y_base - y_top;

    // Calculate the y axis units of measurement
    let (y_axis_max_value, y_axis_step) = axis_max(highest_val);
    let y_unit = y_length / y_axis_max_value;
    let y_unit_step = y_unit * y_axis_step;

    // * Draw y axis marker lines *

    // Determine the width of the widest y axis marker
    let mut y_axis_marker_largest_width = 0.0;
    ctx.set_font(&format!("{}pt serif", y_axis_marker_font_height));
    let mut i = y_base;
    while i >= y_top {
        let marker_label = &format!("{} ", ((y_base - i) / y_unit).round());
        let marker_metrics = ctx.measure_text(&marker_label).unwrap();
        let y_axis_marker_width = marker_metrics.width();
        if y_axis_marker_width > y_axis_marker_largest_width {
            y_axis_marker_largest_width = y_axis_marker_width;
        }
        i -= y_unit_step;
    }
    if DEBUG {
        web_sys::console::log_1(&format!(
            "Widest Y axis marker: {} pixels", &y_axis_marker_largest_width).into()
        );
    }

    let y_marker_x = graph_space_left + y_axis_marker_largest_width;

    if DEBUG {
        web_sys::console::log_1(&format!(
            "y_marker_x: {}", &y_marker_x).into()
        );

        // Draw the Y axis marker labels alignment line
        let dash = Array::new();
        dash.push(&"1".into());
        dash.push(&"3".into());
        ctx.save();
        ctx.set_line_width(1.0);
        ctx.set_stroke_style(&"grey".into());
        ctx.set_line_dash(&dash.into());

        ctx.begin_path();
        ctx.move_to(y_marker_x, graph_space_top);
        ctx.line_to(y_marker_x, graph_space_bottom);
        ctx.stroke();
        ctx.restore();
    }

    // Draw the Y axis marker lines and labels
    ctx.set_stroke_style(&"rgb(220, 220, 220)".into());
    ctx.set_fill_style(&"black".into());
    ctx.set_font(&format!("{}pt serif", y_axis_marker_font_height));
    ctx.set_text_align(&"right");
    let mut i = y_base;
    while i >= y_top {
        let marker_label = &format!("{} ", ((y_base - i) / y_unit).round());
        let marker_metrics = ctx.measure_text(&marker_label).unwrap();
        let y_axis_marker_width = marker_metrics.width();
        ctx.begin_path();
        ctx.move_to(y_marker_x - y_axis_marker_width, i);
        ctx.line_to(graph_space_right - y_axis_marker_largest_width, i);
        ctx.stroke();
        ctx.fill_text(marker_label, y_marker_x, i - (area_root * 0.003));
        i -= y_unit_step;
    }

    // Calculate the bar size, gap, and centering based upon the number of bars
    let num_bars = item_counts.len() as f64;
    let horiz_size = graph_space_width - (2.0 * y_axis_marker_largest_width);
    let bar_space = horiz_size / num_bars;
    let bar_width = bar_space * 0.6; // Bars take 60% of the space, gaps between take 40%
    let bar_gap = bar_space - bar_width;
    let mut bar_left = y_marker_x + (bar_gap / 2.0); // There is "1/2 a bar gap" space between the y axis edge, and the first bar
    let axis_left = y_marker_x;
    let axis_right = graph_space_right - y_axis_marker_largest_width;

    // Draw simple bar graph using the category data
    let mut hue = palette;
    ctx.set_line_width(bar_border);
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
        ctx.move_to(bar_left, y_base);
        ctx.line_to(bar_left + bar_width - bar_border, y_base);
        ctx.line_to(bar_left + bar_width - bar_border, y_base - bar_height);
        ctx.line_to(bar_left, y_base - bar_height);
        ctx.close_path();
        ctx.fill();
        ctx.stroke();
        ctx.set_fill_style(&"black".into());

        // Draw the bar label horizontally centered
        ctx.set_font(&format!("{}pt serif", x_axis_label_font_height));
        let text_left = bar_left + bar_width / 2.0;
        ctx.fill_text(label, text_left, bar_label_y);

        if DEBUG {
            // Draw vertical center line down each bar
            let dash = Array::new();
            dash.push(&"1".into());
            dash.push(&"3".into());
            ctx.save();
            ctx.set_line_width(1.0);
            ctx.set_stroke_style(&"grey".into());
            ctx.set_line_dash(&dash.into());
            ctx.begin_path();
            ctx.move_to(text_left, graph_space_bottom);
            ctx.line_to(text_left, graph_space_top);
            ctx.stroke();
            ctx.restore();
        }

        // Draw the item count centered above the top of the bar
        ctx.set_font(&format!("{}pt serif", x_count_font_height));
        ctx.fill_text(
            &format!("{}", num),
            text_left,
            y_base - bar_height - x_axis_caption_text_gap,
        );
        bar_left += bar_gap + bar_width;
    }

    // Draw axis
    ctx.set_line_width(axis_thickness);
    ctx.begin_path();
    ctx.move_to(axis_right, y_base);
    ctx.line_to(axis_left, y_base);
    ctx.line_to(axis_left, y_top);
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
    ctx.set_fill_style(&"black".into());
    ctx.set_text_align(&"center");
    let title_x = graph_space_left + (graph_space_width / 2.0);
    let title_y = top_space_top + (top_space_height / 2.0) + (title_font_height / 2.0);
    ctx.fill_text(title, title_x, title_y);

    // Draw Y axis caption
    // Info on how to rotate text on the canvas:
    //   https://newspaint.wordpress.com/2014/05/22/writing-rotated-text-on-a-javascript-canvas/
    let y_axis_caption = &data.YAxisLabel;
    let y_axis_caption_string = format!("italic {}pt serif", y_axis_caption_font_height);
    let y_axis_caption_metrics = ctx.measure_text(&y_axis_caption_string).unwrap();
    let y_axis_caption_width = y_axis_caption_metrics.width().round();
    let spin_x = (left_space_left + (left_space_width / 2.0)) + y_axis_caption_font_height;
    let spin_y = (canvas_height / 2.0) - axis_thickness - x_axis_label_font_height;
    ctx.save();
    ctx.translate(spin_x, spin_y);
    ctx.rotate(3.0 * std::f64::consts::PI / 2.0);
    ctx.set_font(&y_axis_caption_string);
    ctx.set_fill_style(&"black".into());
    ctx.set_text_align(&"center");
    ctx.fill_text(
        y_axis_caption,
        0.0,
        0.0
    );
    ctx.restore();

    if DEBUG {
        web_sys::console::log_1(&format!("y_axis_caption_width: {}", &y_axis_caption_width).into());
    }

    // Draw X axis caption
    let x_axis_caption = &data.XAxisLabel;
    let x_axis_caption_string = format!("italic {}pt serif", x_axis_caption_font_height);
    let x_axis_caption_metrics = ctx.measure_text(&x_axis_caption_string).unwrap();
    let x_axis_caption_width = x_axis_caption_metrics.width().round();
    let x_axis_caption_x = (graph_space_left + (graph_space_width / 2.0));
    let x_axis_caption_y = bottom_space_top + (bottom_space_height / 2.0);
    ctx.set_font(&x_axis_caption_string);
    ctx.set_fill_style(&"black".into());
    ctx.set_text_align(&"center");
    ctx.fill_text(
        x_axis_caption,
        x_axis_caption_x,
        x_axis_caption_y
    );

    if DEBUG {
        web_sys::console::log_1(&format!("graph_space_left: {}", &graph_space_left).into());
        web_sys::console::log_1(&format!("graph_space_width: {}", &graph_space_width).into());
        web_sys::console::log_1(&format!("x_axis_caption: {}", &x_axis_caption).into());
        web_sys::console::log_1(&format!("x_axis_caption_font_height: {}", &x_axis_caption_font_height).into());
        web_sys::console::log_1(&format!("x_axis_caption_width: {}", &x_axis_caption_width).into());
        web_sys::console::log_1(&format!("x_axis_caption_x: {}", &x_axis_caption_x).into());
        web_sys::console::log_1(&format!("bottom_space_top: {}", &bottom_space_top).into());
        web_sys::console::log_1(&format!("bottom_space_height: {}", &bottom_space_height).into());
        web_sys::console::log_1(&format!("x_axis_caption_y: {}", &x_axis_caption_y).into());
    }

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
