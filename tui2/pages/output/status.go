package output

import "github.com/streamingfast/substreams/tui2/styles"

func (o *Output) renderStatus() string {
	/*
		[ BACKPROCESSING ]  Press 'p' to see progress.
		[ STREAMING, 5% COMPLETED ]
		[ ERROR STREAMING ]
		[ STREAM FINISHED ]
		[ 209 MB read / 0 B writen ]                 [ 60 workers ]
		[ trace id: dbc0142417451be49ff351b794f58661 ]
	*/
	status := "READY"

	return styles.StatusBar.Render(styles.StatusBarValue.Render(status))
}
