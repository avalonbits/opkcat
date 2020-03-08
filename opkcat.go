package opkcat

import (
	"bytes"
	"io/ioutil"
	"os"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

var opkEnd = []byte(".opk")

// SourceList returns a list of URLs of known opk files
func SourceList(markdown string) []string {
	f, err := os.Open(markdown)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	buf, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	mdParser := parser.New()
	node := mdParser.Parse(buf)

	opks := make([]string, 0, 32)
	ast.WalkFunc(node, ast.NodeVisitorFunc(func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		// We look for links in the page that end with .opk
		link, ok := node.(*ast.Link)
		if !ok {
			return ast.GoToNext
		}
		if !bytes.HasSuffix(link.Destination, opkEnd) {
			return ast.GoToNext
		}

		opks = append(opks, string(link.Destination))
		return ast.GoToNext
	}))
	return opks
}
