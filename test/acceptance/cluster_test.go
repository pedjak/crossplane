package main

//func Test(t *testing.T) {
//
//	c := &cluster{cli: "kubectl"}
//	//fmt.Println("foo")
//	res, err := c.GetAndFilterResourceByJq("nopresources.nop.example.org", "nop-example", "test-36", ".status.conditions[] | select(.type==\"Synced\").status")
//	//err := c.createNamespace("test-2")
//	if err != nil {
//		fmt.Println(err)
//	}
//	var s string
//	err = json.Unmarshal([]byte(res), &s)
//	if err != nil {
//		fmt.Println(err)
//	}
//	fmt.Println(s)
//}

//func Test2(t *testing.T) {
//	script.Exec("ls").Exec("cat").Stdout()
//}
