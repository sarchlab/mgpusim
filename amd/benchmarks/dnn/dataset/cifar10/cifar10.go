// Package cifar10 provides an interface to read the cifar-10 dataset.
package cifar10

import (
	"encoding/binary"
	"flag"
	"os"
	"path"
	"runtime"
	"strconv"
)

var cifar10DataFolder = flag.String("cifar10-data-folder", "",
	"Specifies where the mnist data is located at.")

// The DataSet can provide Cifar-10 data.
type DataSet struct {
	imageFile  *os.File
	numImages  int
	index      int
	channelNum int
	dataNum    int
}

func (d *DataSet) initTrain() {
	d.numImages = 10000
	d.index = 0
	d.channelNum = 0
	d.dataNum = 1
}

func (d *DataSet) initTest() {
	d.numImages = 10000
	d.index = 0
	d.channelNum = 0
	d.dataNum = 5
}

func (d *DataSet) openTrainingFile() {
	fileDir := getDirPath()
	imageFileName := path.Join(
		fileDir + "/cifar-10-batches-bin/data_batch_" +
			strconv.Itoa(d.dataNum) + ".bin")
	d.imageFile = openBin(imageFileName)
}

func (d *DataSet) openTestFile() {
	fileDir := getDirPath()
	imageFileName := path.Join(fileDir + "/cifar-10-batches-bin/test_batch.bin")
	d.imageFile = openBin(imageFileName)
}

// HasNext returns true if there are more images in the dataset.
func (d *DataSet) HasNext() bool {
	flag := true
	if d.index == d.numImages && d.dataNum != 5 {
		d.index = 0
		d.dataNum++
		d.openTrainingFile()
	} else if d.index == d.numImages && d.dataNum == 5 {
		flag = false
	}

	return flag
}

func (d *DataSet) isFirstChannel() bool {
	return d.channelNum == 1
}

// Next returns the next image and label. It panics if there is no more images
// in the dataset.
func (d *DataSet) Next() (image []byte, label byte) {
	var err error

	var imageArray [1024]byte

	if d.channelNum == 0 {
		err = binary.Read(d.imageFile, binary.BigEndian, &label)
		dieOnErr(err)
		d.channelNum++
	} else if d.channelNum == 1 {
		d.channelNum++
	} else if d.channelNum == 2 {
		d.channelNum = 0
		d.index++
	}

	err = binary.Read(d.imageFile, binary.BigEndian, &imageArray)
	dieOnErr(err)

	return imageArray[:], label
}

func getDirPath() string {
	if *cifar10DataFolder != "" {
		return *cifar10DataFolder
	}

	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic("impossible")
	}

	fileDir := path.Dir(filename) + "/data/cifar-10-batches-bin/"
	return fileDir
}

func openBin(filename string) *os.File {
	file, err := os.Open(filename)
	if err != nil {
		panic(filename + " not found.")
	}
	return file
}

func dieOnErr(err error) {
	if err != nil {
		panic(err)
	}
}
