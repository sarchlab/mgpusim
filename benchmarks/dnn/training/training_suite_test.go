package training

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_training_test.go" -package $GOPACKAGE -self_package $GOPACKAGE -write_package_comment=false gitlab.com/akita/dnn/training LossFunction,DataSource
//go:generate mockgen -destination "mock_tensor_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/dnn/tensor Tensor,Operator
//go:generate mockgen -destination "mock_layers_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/dnn/layers Layer
//go:generate mockgen -destination "mock_optimization_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/dnn/training/optimization Alg

func TestTraining(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Training Suite")
}
