# S3 Benchmark

Original based off https://github.com/dvassallo/s3-benchmark

## Credentials

This tool needs AWS credentials with full S3 permissions. If you run this on EC2, it will automatically use your EC2 instance profile. Otherwise it will try to find the credentials [from the usual places](https://aws.amazon.com/blogs/security/a-new-and-standardized-way-to-manage-credentials-in-the-aws-sdks/).

**USE environment variables**: AWS_ACCESS_KEY_ID= AWS_SECRET_ACCESS_KEY= ./s3-mailbench ...

## Run

In order to upload new objects for the test, you need to have one or multiple public-inbox
accessible to s3-mailbench.

### Checking out public-inbox

LKML (Linux Kernel Mailing List) are great because they represent a good subset of email
workloads
```
git clone https://erol.kernel.org/lkml/git/7 lkml-7 
```

### Run the test
```
AWS_ACCESS_KEY_ID="YOURACCESSKEY" AWS_SECRET_ACCESS_KEY="YOURSECRETKEY" ./s3-mailbench -r lklm-7 \
	--max-messages 50000 -w 1,4,8,16,32 \
	--bucket-name YOURBUCKET \
	--endpoint https://storage.gra.cloud.ovh.net \
	--region "gra" \
	--upload --download --clean
```
This test will run the following:
  - Using a single HTTP connection:
  	- upload 50_000 objects to S3 using s3bench/ prefix and measure latency / bandwidth
  	- download 50_000 objects and measure latency / bandwidth
  - Using 4 simultaneous HTTP connections:
    - Upload 50_000 objects
    - Download 50_000 objects
  - ...
  - Delete the 50_000 objects created.


### Output
This test was done using OVH's object cloud storage, with 5ms network latency.

```
+--------+--------------+-----+-----+-----+-----+-----+------+------+
|  TEST  |  THROUGHPUT  | AVG | P25 | P50 | P75 | P90 | P99  | MAX  |
+--------+--------------+-----+-----+-----+-----+-----+------+------+
| PUT 1  | 27.73 KiB/s  | 290 | 167 | 222 | 333 | 562 | 1030 | 5408 |
| GET 1  | 164.48 KiB/s |  49 |  32 |  38 |  47 |  72 |  252 | 2260 |
| PUT 4  | 122.93 KiB/s | 261 | 154 | 205 | 300 | 505 |  829 | 2597 |
| GET 4  | 692.30 KiB/s |  46 |  30 |  36 |  45 |  65 |  247 | 1574 |
| PUT 8  | 243.18 KiB/s | 264 | 159 | 211 | 305 | 493 |  777 | 2485 |
| GET 8  | 1.34 MiB/s   |  43 |  28 |  35 |  44 |  66 |  199 | 1319 |
| PUT 16 | 423.21 KiB/s | 302 | 176 | 241 | 373 | 598 |  819 | 2668 |
| GET 16 | 2.44 MiB/s   |  50 |  29 |  37 |  50 |  76 |  259 | 1894 |
| PUT 32 | 841.70 KiB/s | 304 | 169 | 232 | 372 | 612 |  992 | 2119 |
| GET 32 | 4.28 MiB/s   |  45 |  26 |  34 |  45 |  74 |  197 | 2267 |
| DEL 8  | 0.03 KiB/s   | 305 | 170 | 243 | 391 | 600 |  791 | 1861 |
+--------+--------------+-----+-----+-----+-----+-----+------+------+
```

In each test, the numbers represent the average time to operation completion
in milliseconds. Results are binned by percentage of occurence in percent.


## License

This project is released under the [MIT License](LICENSE).
