// Package mnist provides an interface to read the MNIST interface.
package mnist

import (
	"compress/gzip"
	"encoding/binary"
	"flag"
	"os"
	"path"
	"runtime"
)

var mnistDataFolder = flag.String("mnist-data-folder", "",
	"Specifies where the mnist data is located at.")

// The DataSet can provide MNIST data.
type DataSet struct {
	imageFile *gzip.Reader
	labelFile *gzip.Reader
	numImages int
	index     int
}

// OpenTrainingFile opens the files for training.
func (d *DataSet) OpenTrainingFile() {
	fileDir := getDirPath()
	imageFileName := path.Join(fileDir + "/train-images-idx3-ubyte.gz")
	labelFileName := path.Join(fileDir + "/train-labels-idx1-ubyte.gz")

	d.imageFile = openGZip(imageFileName)
	d.labelFile = openGZip(labelFileName)

	d.parseNumImages()
}

// OpenTestFile opens the files for training.
func (d *DataSet) OpenTestFile() {
	fileDir := getDirPath()
	imageFileName := path.Join(fileDir + "/t10k-images-idx3-ubyte.gz")
	labelFileName := path.Join(fileDir + "/t10k-labels-idx1-ubyte.gz")

	d.imageFile = openGZip(imageFileName)
	d.labelFile = openGZip(labelFileName)

	d.parseNumImages()
}

// HasNext returns true if there are more images in the dataset.
func (d DataSet) HasNext() bool {
	return d.index < d.numImages
}

// Next returns the next image and label. It panics if there is no more images
// in the dataset.
func (d *DataSet) Next() (image []byte, label byte) {
	var err error

	var imageArray [784]byte
	err = binary.Read(d.imageFile, binary.BigEndian, &imageArray)
	dieOnErr(err)

	err = binary.Read(d.labelFile, binary.BigEndian, &label)
	dieOnErr(err)

	d.index++

	return imageArray[:], label
}

func getDirPath() string {
	if *mnistDataFolder != "" {
		return *mnistDataFolder
	}

	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic("impossible")
	}

	fileDir := path.Dir(filename) + "/data/"
	return fileDir
}

func openGZip(filename string) *gzip.Reader {
	file, err := os.Open(filename)
	if err != nil {
		panic(filename + " not found.")
	}

	gzipReader, err := gzip.NewReader(file)
	dieOnErr(err)

	return gzipReader
}

func (d *DataSet) parseNumImages() {
	var err error
	var labelMagic, imageMagic uint32
	var labelN uint32
	var imageX, imageY, imageN uint32

	err = binary.Read(d.labelFile, binary.BigEndian, &labelMagic)
	dieOnErr(err)

	err = binary.Read(d.labelFile, binary.BigEndian, &labelN)
	dieOnErr(err)

	err = binary.Read(d.imageFile, binary.BigEndian, &imageMagic)
	dieOnErr(err)

	err = binary.Read(d.imageFile, binary.BigEndian, &imageN)
	dieOnErr(err)

	err = binary.Read(d.imageFile, binary.BigEndian, &imageX)
	dieOnErr(err)

	err = binary.Read(d.imageFile, binary.BigEndian, &imageY)
	dieOnErr(err)

	numImageMustMatch(labelN, imageN)

	d.numImages = int(imageN)
	d.index = 0
}

func dieOnErr(err error) {
	if err != nil {
		panic(err)
	}
}

func numImageMustMatch(labelN, imageN uint32) {
	if labelN != imageN {
		panic("number of image mismatch")
	}
}
