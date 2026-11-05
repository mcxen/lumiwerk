module github.com/mcxen/lumiwerk

go 1.27.3

require (
	github.com/abema/go-mp4 v1.7.4
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/ebitengine/purego v0.11.1
	github.com/egoist/mygo v0.7.4
	github.com/evanoberholster/imagemeta v1.1.0
	github.com/gen2brain/go-mpv v0.4.0
	github.com/gen2brain/heic v0.7.2
	github.com/mozillazg/go-pinyin v0.21.0
	github.com/remko/go-mkvparse v0.14.0
	go.etcd.io/bbolt v1.5.0
	golang.org/x/image v0.46.0
)

require (
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	github.com/tinylib/msgp v1.6.3 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/gen2brain/go-mpv => ./third_party/go-mpv
