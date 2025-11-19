package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	e2e "github.com/taoyao-code/iot-server/test/e2e"
)

func main() {
	cfg := e2e.GetConfig()

	deviceID := flag.String("device", cfg.TestDeviceID, "device physical ID")
	portNo := flag.Int("port", 1, "port number to use")
	amount := flag.Int("amount", 100, "charge amount in cent")
	mode := flag.Int("mode", int(e2e.ChargeModeByAmount), "charge mode (1: duration, 2: amount, 3: power, 4: auto-stop)")
	waitTimeout := flag.Duration("wait-timeout", cfg.WaitTimeout, "timeout for waiting device/order status")
	flag.Parse()

	logger := log.New(os.Stdout, "[chargeflow] ", log.LstdFlags|log.Lmicroseconds)

	logger.Printf("Starting charge flow\n  server=%s\n  device=%s\n  port=%d\n  amount=%d\n  mode=%d\n  wait_timeout=%s",
		cfg.ServerURL, *deviceID, *portNo, *amount, *mode, waitTimeout.String())

	ctx := context.Background()
	client := e2e.NewAPIClient(cfg)

	logger.Println("Waiting for device to be online...")
	if err := client.WaitForDeviceOnline(ctx, *deviceID, *waitTimeout); err != nil {
		logger.Fatalf("device not online: %v", err)
	}
	logger.Println("Device is online")

	startReq := &e2e.StartChargeRequest{
		PortNo:     *portNo,
		ChargeMode: e2e.ChargeMode(*mode),
		Amount:     *amount,
	}

	logger.Println("Sending start charge request...")
	startResp, err := client.StartCharge(ctx, *deviceID, startReq)
	if err != nil {
		logger.Fatalf("start charge failed: %v", err)
	}
	orderNo := startResp.OrderNo
	logger.Printf("Start charge accepted, order_no=%s", orderNo)

	logger.Println("Waiting for order status=charging...")
	order, err := client.WaitForOrderStatus(ctx, orderNo, e2e.OrderStatusCharging, *waitTimeout)
	if err != nil {
		logger.Printf("wait for charging returned error: %v", err)
	}
	if order != nil {
		logger.Printf("Order now in status=%s, energy=%.3f kWh, actual_amount=%d cent",
			order.Status, order.EnergyConsumed, order.ActualAmount)
	}

	// Allow some time for charging before stopping
	sleepDuration := 10 * time.Second
	logger.Printf("Sleeping %s before stopping charge...", sleepDuration)
	time.Sleep(sleepDuration)

	logger.Println("Sending stop charge request...")
	if err := client.StopCharge(ctx, *deviceID, *portNo); err != nil {
		logger.Fatalf("stop charge failed: %v", err)
	}
	logger.Println("Stop charge request sent, waiting for completion...")

	order, err = client.WaitForOrderStatus(ctx, orderNo, e2e.OrderStatusCompleted, *waitTimeout)
	if err != nil {
		logger.Fatalf("wait for completed failed: %v", err)
	}

	logger.Printf("Order completed successfully\n  order_no=%s\n  final_status=%s\n  energy=%.3f kWh\n  actual_amount=%d cent",
		order.OrderNo, order.Status, order.EnergyConsumed, order.ActualAmount)

	fmt.Println("Charge flow finished successfully")
}
