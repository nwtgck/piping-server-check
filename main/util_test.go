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

const cachedBinDirName = ".bin"

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

func downloadTarGzAndExtractAndFindByFileName(url string, fileBaseName string) (io.Reader, error) {
	var tarGzResp *http.Response
	tarGzResp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if tarGzResp.StatusCode != 200 {
		return nil, fmt.Errorf("status code of GET %s is %d", url, tarGzResp.StatusCode)
	}
	gzipReader, err := gzip.NewReader(tarGzResp.Body)
	if err != nil {
		return nil, err
	}
	tarReader := tar.NewReader(gzipReader)
	for {
		var tarHeader *tar.Header
		tarHeader, err = tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found", fileBaseName)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(tarHeader.Name) == fileBaseName {
			return tarReader, nil
		}
	}
}

func downloadPipingServerPkgIfNotCached(version string) (binPath string, err error) {
	var rootPath string
	rootPath, err = moduleRootPath()
	if err != nil {
		return
	}
	binDirPath := filepath.Join(rootPath, cachedBinDirName, "piping-server-pkg", version, runtime.GOOS+"-"+runtime.GOARCH)
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
	binUrl := fmt.Sprintf("https://github.com/nwtgck/piping-server-pkg/releases/download/v%s/piping-server-pkg-%s-%s.tar.gz", version, pkgOs, pkgArch)
	var pipingServerBinReader io.Reader
	pipingServerBinReader, err = downloadTarGzAndExtractAndFindByFileName(binUrl, "piping-server")
	if err != nil {
		return
	}
	var binFile *os.File
	binFile, err = os.OpenFile(binPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return
	}
	defer binFile.Close()
	_, err = io.Copy(binFile, pipingServerBinReader)
	return
}

func downloadGoPipingServerIfNotCached(version string) (binPath string, err error) {
	var rootPath string
	rootPath, err = moduleRootPath()
	if err != nil {
		return
	}
	binDirPath := filepath.Join(rootPath, cachedBinDirName, "go-piping-server", version, runtime.GOOS+"-"+runtime.GOARCH)
	if err = os.MkdirAll(binDirPath, 0755); err != nil {
		return
	}
	binPath = filepath.Join(binDirPath, "piping-server")
	if stat, _ := os.Stat(binPath); stat != nil {
		return
	}
	binUrl := fmt.Sprintf("https://github.com/nwtgck/go-piping-server/releases/download/v%s/go-piping-server-%s-%s-%s.tar.gz", version, version, runtime.GOOS, runtime.GOARCH)
	var pipingServerBinReader io.Reader
	pipingServerBinReader, err = downloadTarGzAndExtractAndFindByFileName(binUrl, "go-piping-server")
	if err != nil {
		return
	}
	var binFile *os.File
	binFile, err = os.OpenFile(binPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return
	}
	defer binFile.Close()
	_, err = io.Copy(binFile, pipingServerBinReader)
	return
}
