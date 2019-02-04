package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo" // . "github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/mem/vm"
)

var _ = Describe("CommandProcessor", func() {

	var (
		mockCtrl         *gomock.Controller
		engine           *mock_akita.MockEngine
		driver           *mock_akita.MockPort
		dispatcher       *mock_akita.MockPort
		commandProcessor *CommandProcessor
		toDriver         *mock_akita.MockPort
		toDispatcher     *mock_akita.MockPort
		cu               []*mock_akita.MockPort
		toCU             []*mock_akita.MockPort
		vmModules        []*mock_akita.MockPort
		toVMModules      []*mock_akita.MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = mock_akita.NewMockEngine(mockCtrl)

		driver = mock_akita.NewMockPort(mockCtrl)
		dispatcher = mock_akita.NewMockPort(mockCtrl)
		toDriver = mock_akita.NewMockPort(mockCtrl)
		toDispatcher = mock_akita.NewMockPort(mockCtrl)
		commandProcessor = NewCommandProcessor("commandProcessor", engine)
		commandProcessor.numCUs = 10
		commandProcessor.numVMUnits = 11
		commandProcessor.ToDispatcher = toDispatcher
		commandProcessor.ToDriver = toDriver

		commandProcessor.Dispatcher = dispatcher
		commandProcessor.Driver = driver

		for i := 0; i < int(commandProcessor.numCUs); i++ {

			toCU = append(toCU, mock_akita.NewMockPort(mockCtrl))
			cu = append(cu, mock_akita.NewMockPort(mockCtrl))

			commandProcessor.ToCU = append(commandProcessor.ToCU, akita.NewLimitNumReqPort(commandProcessor, 1))
			commandProcessor.CU = append(commandProcessor.CU, akita.NewLimitNumReqPort(commandProcessor, 1))

			commandProcessor.ToCU[i] = toCU[i]
			commandProcessor.CU[i] = cu[i]
		}

		for i := 0; i < int(commandProcessor.numVMUnits); i++ {

			toVMModules = append(toVMModules, mock_akita.NewMockPort(mockCtrl))
			vmModules = append(vmModules, mock_akita.NewMockPort(mockCtrl))

			commandProcessor.ToVMModules = append(commandProcessor.ToVMModules, akita.NewLimitNumReqPort(commandProcessor, 1))
			commandProcessor.VMModules = append(commandProcessor.VMModules, akita.NewLimitNumReqPort(commandProcessor, 1))

			commandProcessor.ToVMModules[i] = toVMModules[i]
			commandProcessor.VMModules[i] = vmModules[i]
		}

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should forward kernel launching request to Dispatcher", func() {
		req := NewLaunchKernelReq(10,
			driver, commandProcessor.ToDriver)
		req.SetEventTime(10)

		toDispatcher.EXPECT().Send(gomock.AssignableToTypeOf(req))

		commandProcessor.Handle(req)
	})

	It("should delay forward kernel launching request to the Driver", func() {
		req := NewLaunchKernelReq(10,
			dispatcher, commandProcessor.ToDispatcher)

		engine.EXPECT().Schedule(
			gomock.AssignableToTypeOf(&ReplyKernelCompletionEvent{}))

		commandProcessor.Handle(req)
	})

	It("should handle a TLB shootdown request from the Driver and send a pipeline drain to CU", func() {
		vAddr := make([]uint64, 1)
		vAddr[0] = 0x1000

		req := NewShootdownCommand(10, nil, commandProcessor.ToDriver, vAddr, 1)
		req.SetEventTime(10)

		for i := 0; i < int(commandProcessor.numCUs); i++ {
			reqDrain := NewCUPipelineDrainReq(10, commandProcessor.ToCU[i], commandProcessor.CU[i])
			toCU[i].EXPECT().Send(gomock.AssignableToTypeOf(reqDrain))
		}

		commandProcessor.Handle(req)
	})

	It("should handle a CU drain ack from the CU and send a invalidation to all VM units", func() {

		vAddr := make([]uint64, 1)
		vAddr[0] = 0x1000
		shootDownreq := NewShootdownCommand(8, nil, commandProcessor.ToDriver, vAddr, 1)
		commandProcessor.curShootdownRequest = shootDownreq

		req := NewCUPipelineDrainRsp(10, nil, commandProcessor.ToCU[0])
		req.drainPipelineComplete = true
		req.SetEventTime(10)

		commandProcessor.numCURecvdAck = commandProcessor.numCUs - 1

		for i := 0; i < int(commandProcessor.numVMUnits); i++ {

			reqShootdown := vm.NewPTEInvalidationReq(10, commandProcessor.ToVMModules[i], commandProcessor.VMModules[i], shootDownreq.pID, shootDownreq.vAddr)
			toVMModules[i].EXPECT().Send(gomock.AssignableToTypeOf(reqShootdown))
		}

		commandProcessor.Handle(req)

	})

	FIt("should handle a VM invalidation done from the VM units and send a ack to driver", func() {

		shootDownRsp := vm.NewInvalidationCompleteRsp(10, nil, commandProcessor.ToVMModules[0], "vm")
		shootDownRsp.InvalidationDone = true
		commandProcessor.numVMRecvdAck = commandProcessor.numVMUnits - 1

		shootDownComplete := NewShootdownCompleteCommand(10, commandProcessor.ToDriver, commandProcessor.Driver)
		toDriver.EXPECT().Send(gomock.AssignableToTypeOf(shootDownComplete))

		commandProcessor.Handle(shootDownRsp)

		Expect(commandProcessor.shootDownInProcess).To(BeFalse())
		Expect(commandProcessor.numVMRecvdAck).To(Equal(uint64(0)))
		Expect(commandProcessor.numCURecvdAck).To(Equal(uint64(0)))

	})

})
