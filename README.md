# GopherPods

GopherPods is a community-driven list of podcast episodes that cover the Go programming language and Go related projects.

https://gopherpods.netlify.com

## Add an episode

Add new episodes to the end of the `episodes.json` file.

Be sure to link directly to the show page of the particular episode.

## Develop

Install dependencies

```
go get
```

Build

```
go build
```

Generate site

```
./gopherpods
```

This will generate the `index.html`, `rss.xml` and `atom.xml` files. 


## License

MIT
