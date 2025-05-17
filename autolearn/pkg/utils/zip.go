package utils

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

func Zip(files []string, zipName string) error {
	zipfile, err := os.Create(zipName)
	if err != nil {
		log.WithError(err).Errorf("creat zip file failed, zip name: %s", zipName)
	}
	defer zipfile.Close()

	zw := zip.NewWriter(zipfile)
	defer zw.Close()

	if err != nil {
		log.WithError(err).Errorf("open zip file failed, zip name: %s", zipName)
	}
	for _, path := range files {
		info, _ := os.Stat(path)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			log.WithError(err).Errorf("get file info failed: %s", path)
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			log.WithError(err).Errorf("create header failed: %s", path)
		}
		file, err := os.Open(path)
		if err != nil {
			log.WithError(err).Errorf("open %s failed:", path)
		}
		n, err := io.Copy(writer, file)
		if err != nil {
			log.WithError(err).Errorf("write %s to zip file failed:", path)
		}
		log.Infof("save file to zipfile: %s success, %d", zipName, n)
	}
	return nil
}

func ZipDir(src, dst string) (err error) {
	fw, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fw.Close()

	zw := zip.NewWriter(fw)
	defer func() {
		if err := zw.Close(); err != nil {
			log.Fatalln(err)
		}
	}()

	return filepath.Walk(src, func(path string, fi os.FileInfo, errBack error) (err error) {
		if errBack != nil {
			return errBack
		}

		fh, err := zip.FileInfoHeader(fi)
		if err != nil {
			return
		}

		fh.Name = strings.TrimPrefix(path, string(filepath.Separator))

		if fi.IsDir() {
			fh.Name += "/"
		}

		w, err := zw.CreateHeader(fh)
		if err != nil {
			return
		}

		if !fh.Mode().IsRegular() {
			return nil
		}

		fr, err := os.Open(path)
		if err != nil {
			return
		}
		defer fr.Close()

		n, err := io.Copy(w, fr)
		if err != nil {
			return
		}
		log.Infof("success compress： %s, total %d byte\n", path, n)
		return nil
	})
}

// AppendZip append files to an exist zipfile, and create a new zipFile
func AppendZip(files []string, existZipPath, targetZipPath string) error {
	zipReader, err := zip.OpenReader(existZipPath)
	if err != nil {
		log.WithError(err).Errorf("open: %s zipfile failed", existZipPath)
		return err
	}
	targetFile, err := os.Create(targetZipPath)
	if err != nil {
		log.WithError(err).Errorf("create: %s zipfile failed", targetZipPath)
		return err
	}
	defer targetFile.Close()
	targetZipWriter := zip.NewWriter(targetFile)
	defer targetZipWriter.Close()

	for _, zipItem := range zipReader.File {
		zipItemReader, _ := zipItem.Open()
		header, _ := zip.FileInfoHeader(zipItem.FileInfo())
		header.Name = zipItem.Name
		targetItem, _ := targetZipWriter.CreateHeader(header)
		io.Copy(targetItem, zipItemReader)
	}

	for _, path := range files {
		info, _ := os.Stat(path)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			log.WithError(err).Errorf("get file info failed: %s", path)
			return err
		}
		writer, err := targetZipWriter.CreateHeader(header)
		if err != nil {
			log.WithError(err).Errorf("create header failed: %s", path)
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			log.WithError(err).Errorf("open %s failed:", path)
			return err
		}
		_, err = io.Copy(writer, file)
		if err != nil {
			log.WithError(err).Errorf("write %s to zip file failed:", path)
			return err
		}
	}
	return nil
}

func Unzip(filePath, dst string) (err error) {
	zipFile, err := zip.OpenReader(filePath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	for _, f := range zipFile.File {
		filePath := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(filePath, filepath.Clean(dst)+string(os.PathSeparator)) {
			fmt.Println("invalid file path")
			return errors.New("invalid file path")
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			panic(err)
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return err
		}

		if _, err := io.Copy(dstFile, fileInArchive); err != nil {
			return err
		}

		dstFile.Close()
		fileInArchive.Close()
	}
	return nil
}

func FileIsExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	}
	return true
}
