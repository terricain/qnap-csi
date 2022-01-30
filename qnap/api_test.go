package qnap

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func TestClient_Login(t *testing.T) {
	username := os.Getenv("QNAP_USER")
	password := os.Getenv("QNAP_PASS")
	baseURL := os.Getenv("QNAP_URL")

	c, err := NewClient(username, password, baseURL)
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	if err = c.Login(); err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}
}

func getLoggedInClient() (*Client, error) {
	username := os.Getenv("QNAP_USER")
	password := os.Getenv("QNAP_PASS")
	baseURL := os.Getenv("QNAP_URL")

	c, err := NewClient(username, password, baseURL)
	if err != nil {
		return &Client{}, err
	}

	if err = c.Login(); err != nil {
		return &Client{}, err
	}
	return c, nil
}

func TestClient_GetStoragePoolSubscription (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	_, err = c.GetStoragePoolSubscription(1)
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	_, err = c.GetStoragePoolSubscription(2)
	if err == nil {
		t.Fatal("invalid storage pool should return an error")
	}
}

func TestClient_GetStorageLogicalVolumes (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	_, err = c.GetStorageLogicalVolumes()
	if err != nil {
		t.Fatalf("failed to get logical volumes: %#v", err)
	}
}

func TestClient_GetStorageISCSILuns (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	_, err = c.GetStorageISCSILun(0)
	if err != nil {
		t.Fatalf("failed to get lun info: %#v", err)
	}
}

func TestClient_GetStorageISCSITargetList (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	_, err = c.GetStorageISCSITargetList()
	if err != nil {
		t.Fatalf("failed to get target list: %#v", err)
	}
}

func TestClient_CreateDeleteStorageISCSITarget (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	targetIndex, err := c.CreateStorageISCSITarget("test1", false, false, true)
	if err != nil {
		t.Fatalf("failed to create target: %#v", err)
	}
	err = c.CreateStorageISCSIInitiator(targetIndex, false, "", "", false, "", "")
	if err != nil {
		t.Fatalf("failed to create initiator: %#v", err)
	}

	if err = c.DeleteStorageISCSITarget(targetIndex); err != nil {
		t.Fatalf("failed to delete initiator: %#v", err)
	}
}

func TestClient_CreateDeleteStorageISCSIBlockLun (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	resp, err := c.CreateStorageISCSIBlockLUN("test2", 1, 10, false, 512, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create target: %#v", err)
	}

	for {
		lunInfo, err := c.GetStorageISCSILun(resp.Result)
		if err != nil {
			t.Fatalf("failed to create target: %#v", err)
		}
		if lunInfo.StatusString() == "creating" {
			fmt.Println("lun still creating")
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	if err = c.DeleteStorageISCSIBlockLUN(resp.Result, false); err != nil {
		t.Fatalf("failed to delete initiator: %#v", err)
	}
}

func TestClient_CreateAttachStorageISCSITargetBlockLun (t *testing.T) {
	c, err := getLoggedInClient()
	if err != nil {
		t.Fatalf("failed to init client: %#v", err)
	}

	name := "apitest" + RandStringBytes(5)

	targetIndex, err := c.CreateStorageISCSITarget(name, false, false, true)
	if err != nil {
		t.Fatalf("failed to create target: %#v", err)
	}
	err = c.CreateStorageISCSIInitiator(targetIndex, false, "", "", false, "", "")
	if err != nil {
		t.Fatalf("failed to create initiator: %#v", err)
	}

	lunResp, err := c.CreateStorageISCSIBlockLUN(name, 1, 10, false, 512, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create target: %#v", err)
	}

	err = c.AttachStorageISCSITargetLUN(lunResp.Result, targetIndex)
	if err != nil {
		t.Fatalf("failed to create target: %#v", err)
	}

	targetResp, err := c.GetStorageISCSITargetList()
	if err != nil {
		t.Fatalf("failed to create target: %#v", err)
	}
	found := false
	for _, target := range targetResp.Targets {
		if target.TargetIndex == targetIndex {
			found = true
			if len(target.TargetLUNs) != 1 {
				t.Fatal("target luns list is not 1")
			}
			if target.TargetLUNs[0] != lunResp.Result {
				t.Fatalf("target luns %d != %d", lunResp.Result, target.TargetLUNs[0])
			}
		}
	}
	if !found {
		t.Fatal("failed to find target")
	}

	for {
		lunInfo, err := c.GetStorageISCSILun(lunResp.Result)
		if err != nil {
			t.Fatalf("failed to create target: %#v", err)
		}
		if lunInfo.StatusString() == "creating" {
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	if err = c.DeleteStorageISCSITarget(targetIndex); err != nil {
		t.Fatalf("failed to delete initiator: %#v", err)
	}

	if err = c.DeleteStorageISCSIBlockLUN(lunResp.Result, false); err != nil {
		t.Fatalf("failed to delete initiator: %#v", err)
	}
}