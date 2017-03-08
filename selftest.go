package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/buger/goterm"
	"github.com/immesys/hcr"
)

const startaddr = 0x20001100
const extractprog = `connect
power on
sleep 200
h
r
h
loadbin %s, 0
h
r
h
go
Sleep 650
h
mem32 0x20001100, 0x40
power off
exit
`

var testBinPath string
var jlinkBinary string

func run() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, jlinkBinary, "-device", "atsamr21e18", "-if", "swd", "-speed", "2000")
	cmd.Stdin = strings.NewReader(fmt.Sprintf(extractprog, testBinPath))
	rv, err := cmd.CombinedOutput()
	//fmt.Printf("%v %v", err, string(rv))
	return rv, err
}

func decodedRun() ([]uint32, error) {
	raw, err := run()
	if err != nil {
		return nil, err
	}
	rv := make([]uint32, 64)

	bf := bytes.NewBuffer(raw)
	l, err := bf.ReadString('\n')
	offset := 0
	for err == nil {
		l = strings.TrimPrefix(l, "J-Link>")
		pfx := fmt.Sprintf("%08X", startaddr+offset)
		//	fmt.Printf("line: %v (looking for %v)\n", l, pfx)
		if strings.HasPrefix(l, pfx) {
			lx := strings.TrimSpace(l[11:])
			sp := strings.Split(lx, " ")
			if len(sp) != 4 {
				return nil, errors.New("Malformed read line")
			}
			v0, err := strconv.ParseInt(sp[0], 16, 64)
			if err != nil {
				return nil, err
			}
			v1, err := strconv.ParseInt(sp[1], 16, 64)
			if err != nil {
				return nil, err
			}
			v2, err := strconv.ParseInt(sp[2], 16, 64)
			if err != nil {
				return nil, err
			}
			v3, err := strconv.ParseInt(sp[3], 16, 64)
			if err != nil {
				return nil, err
			}
			rv[offset/4] = uint32(v0)
			rv[offset/4+1] = uint32(v1)
			rv[offset/4+2] = uint32(v2)
			rv[offset/4+3] = uint32(v3)
			//fmt.Printf("extracted: %q\n", lx)
			offset += 16
			if offset >= 4*32*2 {
				return rv, nil
			}
		}

		l, err = bf.ReadString('\n')
	}
	return nil, err
}

/*
#define TV_INIT_PRE           0
#define TV_CHIP_ENTERS_MAIN   1
#define TV_HDC_INIT           2
#define TV_HDC_READ           3
#define TV_HDC_TRIG           4
#define TV_TMP_INIT           5
#define TV_TMP_SETACT         6
#define TV_ACC_ADD            7
#define TV_ACC_INIT           8
#define TV_LUX_SAMPLE         9
#define TV_COMPLETE           10
#define TV_TMP_READ           11

#define TV_ACC_DAT            12
#define TV_ACC_DAT_AX         13
#define TV_ACC_DAT_AY         14
#define TV_ACC_DAT_AZ         15
#define TV_ACC_DAT_MX         16
#define TV_ACC_DAT_MY         17
#define TV_ACC_DAT_MZ         18
*/
func cts(code uint32) (string, bool) {
	if code == 0 {
		return goterm.Background("SKIP", goterm.YELLOW), false
	}
	if code == 1 {
		return goterm.Background("PASS", goterm.GREEN), true
	}
	if code == 2 {
		return goterm.Background("FAIL", goterm.RED), false
	}
	return goterm.Background("????", goterm.RED), false
}
func hdc1080_read(v []uint32) (string, string, bool) {
	if v[0] == 1 {
		tmp := float64(int(int16(v[1]&0xFFFF))) / 100.0
		hum := float64(int(int16(v[1]>>16))) / 100.0
		if v[0]&0xFFFF == 0 || v[1]>>16 == 0 {
			return goterm.Background("FAIL", goterm.RED), "readings are zero", false
		}
		return goterm.Background("PASS", goterm.GREEN), fmt.Sprintf("(%.2f C, %.2f%% RH)", tmp, hum), true
	}
	if v[0] == 0 {
		return goterm.Background("SKIP", goterm.YELLOW), "", false
	}
	if v[0] == 2 {
		return goterm.Background("FAIL", goterm.RED), "", false
	}
	return goterm.Background("????", goterm.RED), "test vector is corrupt", false
}
func fxos8700_whoami(v []uint32) (string, string, bool) {
	if v[0] == 1 {
		return goterm.Background("PASS", goterm.GREEN), fmt.Sprintf("(ID=0x%02x)", v[1]), true
	}
	if v[0] == 0 {
		return goterm.Background("SKIP", goterm.YELLOW), "", false
	}
	if v[0] == 2 {
		return goterm.Background("FAIL", goterm.RED), "", false
	}
	return goterm.Background("????", goterm.RED), "test vector is corrupt", false

}
func tmp006_read(v []uint32) (string, string, bool) {
	if v[0] == 1 {
		tval := float64(int(int16(v[1]&0xFFFF))) / 1000.0
		tdie := float64(int(int16(v[1]>>16))) / 100.0
		if v[1]&0xFFFF == 0 || v[1]>>16 == 0 {
			return goterm.Background("FAIL", goterm.RED), "readings are zero", false
		}
		return goterm.Background("PASS", goterm.GREEN), fmt.Sprintf("(TD=%.2f TV=%.2f)", tdie, tval), true
	}
	if v[0] == 0 {
		return goterm.Background("SKIP", goterm.YELLOW), "", false
	}
	if v[0] == 2 {
		return goterm.Background("FAIL", goterm.RED), "", false
	}
	return goterm.Background("????", goterm.RED), "test vector is corrupt", false

}
func fxos8700_acc_data(v []uint32) (string, string, bool) {
	if v[0] == 0 {
		return goterm.Background("SKIP", goterm.YELLOW), "", false
	} else if v[0] == 2 {
		return goterm.Background("FAIL", goterm.RED), "", false
	} else if v[0] != 1 {
		return goterm.Background("????", goterm.RED), "test vector is corrupt", false
	}
	ax := float64(int16(v[3])) / 100.0
	ay := float64(int16(v[5])) / 100.0
	az := float64(int16(v[7])) / 100.0

	return goterm.Background("PASS", goterm.GREEN), fmt.Sprintf("(AX=%.2f AY=%.2f AZ=%.2f)", ax, ay, az), true
}
func fxos8700_mag_data(v []uint32) (string, string, bool) {
	if v[0] == 0 {
		return goterm.Background("SKIP", goterm.YELLOW), "", false
	} else if v[0] == 2 {
		return goterm.Background("FAIL", goterm.RED), "", false
	} else if v[0] != 1 {
		return goterm.Background("????", goterm.RED), "test vector is corrupt", false
	}

	mx := float64(int16(v[9])) / 100.0
	my := float64(int16(v[11])) / 100.0
	mz := float64(int16(v[13])) / 100.0
	return goterm.Background("PASS", goterm.GREEN), fmt.Sprintf("(MX=%.2f, MY=%.2f, MZ=%.2f)", mx, my, mz), true
}
func prog_conn(v []uint32, er error) (string, string, bool) {
	if er != nil || v[0] != 1 {
		return goterm.Background("FAIL", goterm.RED), "", false
	} else {
		return goterm.Background("PASS", goterm.GREEN), "", true
	}
}
func extrapolatedRun(wait chan bool) bool {
	var inrv []uint32
	var err error
	for i := 0; i < 3; i++ {
		//	fmt.Printf("reading test vector attempt %d/3: ", i+1)
		//	os.Stdout.Sync()
		inrv, err = decodedRun()
		if err != nil {
			//	fmt.Printf("could not read vector\n")
			continue
		} else {
			//	fmt.Printf("vector obtained OK\n")
			break
		}
	}
	rv := make([]uint32, 64)
	if err == nil {
		rv = inrv
	}
	allok := true
	msgs := []string{}
	lineformat := "%-22s [%s] %s"

	stat, extra, ok := prog_conn(rv, err)
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "programmer connected", stat, extra))

	stat, ok = cts(rv[1*2])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "SAMR21 oscillators", stat, ""))

	stat, ok = cts(rv[2*2])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "HDC1080 initialize", stat, ""))

	stat, extra, ok = hdc1080_read(rv[3*2:])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "HDC1080 sample", stat, extra))

	stat, ok = cts(rv[5*2])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "TMP006 initialize", stat, ""))

	stat, ok = cts(rv[6*2])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "TMP006 activate", stat, ""))

	stat, extra, ok = tmp006_read(rv[11*2:])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "TMP006 sample", stat, extra))

	stat, ok = cts(rv[8*2])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "FXOS8700 initialize", stat, ""))

	stat, extra, ok = fxos8700_whoami(rv[7*2:])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "FXOS8700 device id", stat, extra))

	stat, extra, ok = fxos8700_acc_data(rv[12*2:])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "FXOS8700 acc sample", stat, extra))

	stat, extra, ok = fxos8700_mag_data(rv[12*2:])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "FXOS8700 mag sample", stat, extra))

	stat, ok = cts(rv[10*2])
	allok = allok && ok
	msgs = append(msgs, fmt.Sprintf(lineformat, "test sequence done", stat, ""))
	<-wait
	for idx, m := range msgs {
		fmt.Printf(" [%-2d] - %s\n", idx, m)
	}
	return allok
}

func checkImage(td string) (bin string, repository string, commit string) {
	img := os.Getenv("CX_IMAGE")
	if img == "" {
		fmt.Printf("You must set $CX_IMAGE to a file or one of:\n")
		for _, e := range fwRegistry {
			fmt.Printf(" - %q   # %s\n", e.Name, e.Description)
		}
		os.Exit(1)
	}
	if strings.HasPrefix(img, "/") {
		//This is a file
		if _, err := os.Stat(img); os.IsNotExist(err) {
			fmt.Printf("%s does not exist\n", img)
			os.Exit(1)
		}
		repository = os.Getenv("CX_REPOSITORY")
		commit = os.Getenv("CX_COMMIT")
		if repository == "" || commit == "" {
			fmt.Printf("If using a file, $CX_REPOSITORY and $CX_COMMIT must be set")
			os.Exit(1)
		}
		return img, repository, commit
	}
	for _, e := range fwRegistry {
		if e.Name == img {
			ppath := filepath.Join(td, "payload.bin")
			dat, err := Asset(e.Asset)
			if err != nil {
				panic(err)
			}
			if err := ioutil.WriteFile(ppath, dat, 0666); err != nil {
				panic(err)
			}
			return ppath, e.Repository, e.Commit
		}
	}
	fmt.Printf("$CX_IMAGE=%q is not a valid builtin image (if you meant to use a file, use the full path)\n", img)
	os.Exit(1)
	return
}

//go:generate go-bindata -o assets.go assets/
func main() {

	td, err := ioutil.TempDir("", "hamilton-cx")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	img, repo, commit := checkImage(td)
	//populate the testbin
	dat, err := Asset("assets/testprog.bin")
	if err != nil {
		panic(err)
	}
	testBinPath = filepath.Join(td, "test.bin")
	if err := ioutil.WriteFile(testBinPath, dat, 0666); err != nil {
		panic(err)
	}
	//populate JLinkExe
	dat, err = Asset("assets/JLinkExe")
	if err != nil {
		panic(err)
	}
	jlinkBinary = filepath.Join(td, "JLinkExe")
	if err := ioutil.WriteFile(jlinkBinary, dat, 0700); err != nil {
		panic(err)
	}

	if os.Getenv("CX_DEPLOYMENT_KEY") == "" {
		fmt.Printf("You must set $CX_DEPLOYMENT_KEY\n")
		os.Exit(1)
	}

	fmt.Printf("HAMILTON CX v1.0\n")
	fmt.Printf("SETTINGS:\n CX_IMAGE=%s\n CX_REPOSITORY=%s\n CX_COMMIT=%s\n\n", os.Getenv("CX_IMAGE"), repo, commit)
	fmt.Printf("HIT RETURN TO TEST/PROGRAM\n")
	for {
		buf := make([]byte, 1)
		n, err := os.Stdin.Read(buf)
		if err != nil || n != 1 {
			os.Exit(1)
		}
		if buf[0] == 10 {
			pr := make(chan bool)
			d := make(chan bool)
			go func() {
				then := time.Now()
				res := extrapolatedRun(pr)
				fmt.Printf("\n Test took %.4vs\n", time.Now().Sub(then))
				d <- res
			}()
			//Test is ~ 1.5 seconds
			fmt.Printf("Executing test: \n")
			for i := 0; i < 75; i++ {
				fmt.Printf("#")
				os.Stdout.Sync()
				time.Sleep(2000 * time.Millisecond / 75)
			}
			fmt.Printf("\n")
			close(pr)
			okay := <-d
			if !okay {
				fmt.Println(goterm.Color("Skipping payload program: test failed", goterm.RED))
				fmt.Printf("HIT RETURN TO TEST/PROGRAM\n")
				continue
			}
			then := time.Now()
			progok, msg := programPayload(td, img, repo, commit, os.Getenv("CX_DEPLOYMENT_KEY"))
			if !progok {
				fmt.Println(goterm.Color(fmt.Sprintf("Payload program failed: %s", msg), goterm.RED))
				fmt.Printf("\nHIT RETURN TO TEST/PROGRAM\n")
			} else {
				fmt.Printf(" Payload program and lockout took %.4vs\n", time.Now().Sub(then))
				fmt.Printf("\nHIT RETURN TO TEST/PROGRAM\n")
			}
		}
	}
}

func mkfactoryblock(td string, moteid int, motetype uint64, symmkey []byte, vk []byte, sk []byte) string {
	out := make([]byte, 1024)
	binary.LittleEndian.PutUint64(out[0:], 0x27c83f60f6b6e7c8)
	binary.LittleEndian.PutUint64(out[8:], uint64(time.Now().UnixNano()/1000))
	//00:12:6d:07:00:00:serial
	out[16] = 0x00
	out[17] = 0x12
	out[18] = 0x6d
	out[19] = 0x07
	out[20] = 0
	out[21] = 0
	out[22] = byte(moteid >> 8)
	out[23] = byte(moteid & 0xFF)

	out[24] = byte(moteid & 0xFF)
	out[25] = byte(moteid >> 8)
	//26 pad
	//27 pad
	binary.LittleEndian.PutUint64(out[28:], motetype)
	//36 .. 47 pad
	//48 to 64 symm key
	copy(out[48:64], symmkey)
	copy(out[64:96], vk)
	copy(out[96:128], sk)
	path := filepath.Join(td, "fblock.bin")
	if err := ioutil.WriteFile(path, out, 0600); err != nil {
		panic(err)
	}
	return path
}

const identifyprog = `connect
power on
sleep 200
erase
mem32 0x0080a00c 1
mem32 0x0080a040 1
mem32 0x0080a044 1
mem32 0x0080a048 1
exit
`

const payloadprog = `connect
h
r
h
loadbin %s, 0
verifybin %s, 0
loadbin %s, 0x3fc00
verifybin %s, 0x3fc00
w2 0x41004000 0xA545
w2 0x41004000 0xA50F
exit
`

// const sealprog = `connect
// h
// r
// h
// loadbin %s, 0x3fc00
// w2 0x41004000 0xA545
// w2 0x41004000 0xA50F
// exit
//`

func identify(ctx context.Context) (bool, string) {
	cmd := exec.CommandContext(ctx, jlinkBinary, "-device", "atsamr21e18", "-if", "swd", "-speed", "2000")
	cmd.Stdin = strings.NewReader(identifyprog)
	rv, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	bf := bytes.NewBuffer(rv)
	l, err := bf.ReadString('\n')
	var w1 int64
	var w2 int64
	var w3 int64
	var w4 int64

	for err == nil {
		if strings.Contains(l, "0080A00C") {
			parts := strings.Split(l, " ")
			w1, err = strconv.ParseInt(parts[2], 16, 64)
			if err != nil {
				return false, ""
			}
		}
		if strings.Contains(l, "0080A040") {
			parts := strings.Split(l, " ")
			w2, err = strconv.ParseInt(parts[2], 16, 64)
			if err != nil {
				return false, ""
			}
		}
		if strings.Contains(l, "0080A044") {
			parts := strings.Split(l, " ")
			w3, err = strconv.ParseInt(parts[2], 16, 64)
			if err != nil {
				return false, ""
			}
		}
		if strings.Contains(l, "0080A048") {
			parts := strings.Split(l, " ")
			w4, err = strconv.ParseInt(parts[2], 16, 64)
			if err != nil {
				return false, ""
			}
		}
		l, err = bf.ReadString('\n')
	}
	if w1 == 0 || w2 == 0 || w3 == 0 || w4 == 0 {
		return false, ""
	}
	return true, fmt.Sprintf("%08x%08x%08x%08x", w1, w2, w3, w4)
}

func programPayload(td string, imgfile string, repo string, commit string, depkey string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, deviceid := identify(ctx)
	if !ok {
		return false, "could not read device ID"
	}
	cl, err := hcr.GetHCRClient()
	if err != nil {
		return false, fmt.Sprintf("could not connect to Hamilton Commissioning Registry: %v", err)
	}

	//Get mote ID
	moteidrv, err := cl.GetMoteID(context.Background(), &hcr.GetMoteIDParams{
		DeviceId: deviceid,
		Auth: &hcr.Auth{
			DeploymentSecret: depkey,
			UserSecret:       os.Getenv("CX_USER_SECRET"),
		},
	})
	if err != nil {
		return false, fmt.Sprintf("could not obtain mote ID from HCR: %s", err)
	}
	if !moteidrv.Status.Okay {
		return false, fmt.Sprintf("could not obtain mote ID from HCR: %s", moteidrv.Status.Message)
	}

	if os.Getenv("CX_ASSIGN_DEPLOYMENT") != "" && os.Getenv("CX_USER_SECRET") != "" {
		rv, err := cl.CreateDeployment(context.Background(), &hcr.CreateDeploymentParams{
			Auth: &hcr.Auth{
				UserSecret: os.Getenv("CX_USER_SECRET"),
			},
			Name: os.Getenv("CX_ASSIGN_DEPLOYMENT"),
		})
		if err != nil {
			panic(err)
		}
		if !rv.Status.Okay {
			fmt.Printf("deployment create result: %v\n", rv.Status.Message)
		} else {
			fmt.Printf("deployment is new\n READ KEY : %s\n WRITE KEY: %s\n", rv.ReadKey, rv.WriteKey)
		}

		rvb, err := cl.BindMote(context.Background(), &hcr.BindMoteParams{
			Auth: &hcr.Auth{
				UserSecret: os.Getenv("CX_USER_SECRET"),
			},
			Deployment: os.Getenv("CX_ASSIGN_DEPLOYMENT"),
			Moteid:     moteidrv.Id,
		})
		if err != nil {
			panic(err)
		}
		if !rvb.Status.Okay {
			fmt.Printf("deployment bind result  : %v\n", rvb.Status.Message)
		} else {
			fmt.Printf("deployment bind ok\n")
		}
	}

	if os.Getenv("CX_USER_SECRET") != "" {
		return false, fmt.Sprintf("skipped (ID registered 0x%04x)", moteidrv.Id)
	}

	//Create instance
	cir, err := cl.CreateInstance(context.Background(), &hcr.CreateInstanceParams{
		Auth: &hcr.Auth{
			DeploymentSecret: depkey,
		},
		DeviceId:   deviceid,
		Repository: repo,
		Commit:     commit,
		Motetype:   0x3c,
		Extradata:  os.Getenv("CX_EXTRADATA"),
	})
	if err != nil {
		return false, fmt.Sprintf("could not create instance in HCR: %s", err)
	}
	if !cir.Status.Okay {
		return false, fmt.Sprintf("could not create instance in HCR: %s", cir.Status.Message)
	}

	fblock := mkfactoryblock(td, int(moteidrv.Id), 0x3C, cir.AESK, cir.Ed25519VK, cir.Ed25519SK)
	_ = fblock

	fmt.Printf("CREATED INSTANCE:\n")
	fmt.Printf(" Mote ID   : 0x%04x\n", moteidrv.Id)
	fmt.Printf(" Mote MAC  : %s\n", moteidrv.Mac)
	fmt.Printf(" Device ID : %s\n", deviceid)
	fmt.Printf(" Ed25519 VK: %s\n", base64.URLEncoding.EncodeToString(cir.Ed25519VK))
	fmt.Printf("Programming and enabling device lockout\n")
	cmd := exec.CommandContext(ctx, jlinkBinary, "-device", "atsamr21e18", "-if", "swd", "-speed", "2000")
	cmd.Stdin = strings.NewReader(fmt.Sprintf(payloadprog, imgfile, imgfile, fblock, fblock))
	rv, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	bf := bytes.NewBuffer(rv)
	l, err := bf.ReadString('\n')
	ver := 0
	for err == nil {
		if strings.Contains(l, "Verify successful.") {
			ver++
		}
		l, err = bf.ReadString('\n')
	}

	if ver != 2 {
		return false, "payload verify failed"
	}

	return true, ""

}
