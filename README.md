# cx - Hamilton Commissioning Tool

To use this, download the `cx` binary from the releases page.

To list the available built-in firmware images, run

```
$ unset CX_IMAGE
$ ./cx
You must set $CX_IMAGE to a file or one of:
 - "3c-qfw-2.0-30s"   # Hamilton-3C, v2.0 30s interval
 - "3c-qfw-2.0-10s"   # Hamilton-3C, v2.0 10s interval
```

To flash one of these images onto a mote that is already part of a deployment, you would do:

```
$ export CX_IMAGE=3c-qfw-2.0-10s
$ export CX_DEPLOYMENT_KEY=<your write key>
$ ./cx
```

To use this tool to register brand new motes with a deployment, you would do

```
export CX_USER_SECRET=<your admin key>
export CX_DEPLOYMENT_KEY=unused
export CX_IMAGE=3c-qfw-2.0-10s
export CX_ASSIGN_DEPLOYMENT=<your deployment name>
$ ./cx
```

If the deployment is new, it will print the deployment keys, like

```
deployment is new
 READ KEY : qTNcZ1-iNSElXSPNSpB6AQ6bhoo7xZ2v
 WRITE KEY: B1AAVgvaJTt8aszuzaDrWOxfeLMGRaFc
```

Write these down, it is not possible to recover them if you lose them.

To decode hamilton messages, please see [the readme in the HCR library](https://github.com/immesys/hcr)
