package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

func moduleRootPath() (string, error) {
	dirPath, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		goModFile := filepath.Join(dirPath, "go.mod")
		if stat, _ := os.Stat(goModFile); stat != nil {
			return dirPath, nil
		}
		dirPath = filepath.Join(dirPath, "..")
	}
}

func downloadPipingServerPkgIfNotCached(version string) (binPath string, err error) {
	var rootPath string
	rootPath, err = moduleRootPath()
	if err != nil {
		return
	}
	binDirPath := filepath.Join(rootPath, ".bin", "piping-server-pkg", version, runtime.GOOS+"-"+runtime.GOARCH)
	if err = os.MkdirAll(binDirPath, 0755); err != nil {
		return
	}
	binPath = filepath.Join(binDirPath, "piping-server")
	if stat, _ := os.Stat(binPath); stat != nil {
		return
	}
	var pkgOs string
	switch runtime.GOOS {
	case "linux":
		pkgOs = "linuxstatic"
	case "darwin":
		pkgOs = "mac"
	default:
		err = fmt.Errorf("%s not supported", runtime.GOOS)
		return
	}
	var pkgArch string
	switch runtime.GOARCH {
	case "amd64":
		pkgArch = "x64"
	case "arm64":
		pkgArch = "arm64"
	default:
		err = fmt.Errorf("%s not supported", runtime.GOARCH)
		return
	}
	var tarGzResp *http.Response
	binUrl := fmt.Sprintf("https://github.com/nwtgck/piping-server-pkg/releases/download/v%s/piping-server-pkg-%s-%s.tar.gz", version, pkgOs, pkgArch)
	tarGzResp, err = http.Get(binUrl)
	if err != nil {
		return
	}
	var gzipReader io.Reader
	gzipReader, err = gzip.NewReader(tarGzResp.Body)
	tarReader := tar.NewReader(gzipReader)
	for {
		var tarHeader *tar.Header
		tarHeader, err = tarReader.Next()
		if err == io.EOF {
			return "", fmt.Errorf("piping-server binary not found")
		}
		if err != nil {
			return
		}
		if filepath.Base(tarHeader.Name) == "piping-server" {
			var binFile *os.File
			binFile, err = os.OpenFile(binPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return
			}
			defer binFile.Close()
			_, err = io.Copy(binFile, tarReader)
			return
		}
	}
}
