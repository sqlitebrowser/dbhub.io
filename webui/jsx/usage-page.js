const React = require("react");
const ReactDOM = require("react-dom");

import ButtonGroup from "react-bootstrap/ButtonGroup";
import ToggleButton from "react-bootstrap/ToggleButton";
import Plotly from "plotly.js-basic-dist";
import createPlotlyComponent from "react-plotly.js/factory";
const Plot = createPlotlyComponent(Plotly);

function ApiUsage() {
	const [selectedValue, setSelectedValue] = React.useState("num_calls");
	const [selectedTime, setSelectedTime] = React.useState("daily");
	const [data, setData] = React.useState(null);

	// Exit early if there is no data yet
	if (apiUsageData === null) {
		return <div className="alert alert-info" role="alert">We did not get any API calls from you recently.</div>;
	}

	// Define available plots
	const availableValues = [
		{
			label: "Number of calls",
			value: "num_calls",
			unit: "#",
		},
		{
			label: "Execution time",
			value: "runtime",
			unit: "ms",
		},
		{
			label: "Incoming traffic",
			value: "request_size",
			unit: "Bytes",
		},
		{
			label: "Outgoing traffic",
			value: "response_size",
			unit: "Bytes",
		},
	];
	const availableTimes = [
		{
			label: "Daily",
			value: "daily",
		},
		{
			label: "Monthly",
			value: "monthly",
		},
	];

	// Helper functions for getting details for current settings
	const currentValueUnit = () => availableValues.find(p => p.value === selectedValue).unit;
	const currentTimeLabel = () => availableTimes.find(p => p.value === selectedTime).label;

	// Update plot data when settings change
	React.useEffect(() => {
		let xData = [];
		let yDataBar = [];
		if (selectedTime === "monthly") {
			// Aggregate the daily data provided by the server to one group per month
			const monthlyData = apiUsageData.reduce((monthlyData, day) => {
				const month = new Date(day.date).getUTCFullYear() + "-" + String(new Date(day.date).getUTCMonth() + 1).padStart(2, "0");
				monthlyData[month] = (monthlyData[month] || 0) + day[selectedValue];
				return monthlyData;
			}, {});

			// Map the monthly data to the format Plotly expects
			xData = Object.keys(monthlyData).sort();
			yDataBar = Object.keys(monthlyData).sort().map(month => monthlyData[month]);
		} else if (selectedTime === "daily") {
			// Map daily data provided by the server to the format Plotly expects
			xData = apiUsageData.map(r => new Date(r.date).getUTCFullYear() + "-" + String(new Date(r.date).getUTCMonth() + 1).padStart(2, "0") + "-" + String(new Date(r.date).getUTCDate()).padStart(2, "0"));
			yDataBar = apiUsageData.map(r => r[selectedValue]);
		}

		// Cumulated the values for the bar graph to generate the data for the line graph
		const yDataLine = yDataBar.map((sum => val => sum += val)(0));

		// Set data for plotting
		setData([{
			x: xData,
			y: yDataBar,
			type: "bar",
			name: currentTimeLabel() + " usage [" + currentValueUnit() + "]",
			width: selectedTime === "monthly" ? undefined : 0.8 * 24 * 60 * 60 * 1000, // 80% of a day for daily data
			xperiod: selectedTime === "monthly" ? "M1" : undefined,
			xperiodalignment: selectedTime === "monthly" ? "middle" : undefined,
		}, {
			x: xData,
			y: yDataLine,
			type: "scatter",
			mode: "lines+markers",
			line: {shape: "hv"},
			yaxis: "y2",
			name: "Cumulative usage [" + currentValueUnit() + "]",
			xperiod: selectedTime === "monthly" ? "M1" : undefined,
			xperiodalignment: selectedTime === "monthly" ? "middle" : undefined,
		}]);
	}, [selectedValue, selectedTime]);

	return (<>
		<ButtonGroup>
			{availableValues.map(p =>
				<ToggleButton
					id={"btn-" + p.value}
					type="radio"
					variant="light"
					name="values"
					value={p.value}
					checked={selectedValue === p.value}
					onChange={e => setSelectedValue(e.currentTarget.value)}
				>{p.label}</ToggleButton>
			)}
		</ButtonGroup>&nbsp;
		<ButtonGroup>
			{availableTimes.map(p =>
				<ToggleButton
					id={"btn-" + p.value}
					type="radio"
					variant="light"
					name="times"
					value={p.value}
					checked={selectedTime === p.value}
					onChange={e => setSelectedTime(e.currentTarget.value)}
				>{p.label}</ToggleButton>
			)}
		</ButtonGroup>

		<Plot
			data={data}
			layout={{
				autosize: true,
				xaxis: {
					type: "date",
					ticks: "outside",
					dtick: selectedTime === "monthly" ? "M1" : undefined,
					ticklabelmode: selectedTime === "monthly" ? "period" : "instant",
					rangeslider: {range: [apiUsageData[0].date, apiUsageData[apiUsageData.length - 1].date]},
					rangeselector: {
						buttons: [
							{
								count: 1,
								label: "1m",
								step: "month",
								stepmode: "backward",
							},
							{
								count: 3,
								label: "3m",
								step: "month",
								stepmode: "backward",
							},
							{
								count: 6,
								label: "6m",
								step: "month",
								stepmode: "backward",
							},
							{
								step: "all",
							},
						]
					},
				},
				yaxis: {
					visible: true,
					shAuth0owline: true,
					ticks: "outside",
					title: currentTimeLabel() + " usage [" + currentValueUnit() + "]",
					rangemode: "tozero",
				},
				yaxis2: {
					side: "right",
					overlaying: "y",
					visible: true,
					showline: true,
					ticks: "outside",
					title: "Cumulative usage [" + currentValueUnit() + "]",
					rangemode: "tozero",
				},
				legend: {
					orientation: "h",
					xanchor: "center",
					yanchor: "bottom",
					y: 1.0,
					x: 0.5,
				},
			}}
			config={{
				watermark: false,
				displayModeBar: false,
			}}
			useResizeHandler={true}
			className="mt-1 w-100"
		/>
	</>);
}

export default function UsagePage() {
	return (<>
		<h3 className="text-center">Usage information</h3>

		<h4>API calls</h4>
		<ApiUsage />
	</>);
}
