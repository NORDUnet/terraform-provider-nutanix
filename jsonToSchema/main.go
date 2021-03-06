package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	glog "log"
	"os"
	"path/filepath"
	"strings"

	"github.com/terraform-providers/terraform-provider-nutanix/jsonToSchema/cfg"
)

//PowerON denotes the exact string for vm-powerstate(on) in structs
const PowerON = "ON"

//PowerOFF denotes the exact string for vm-powerstate(off) in structs
const PowerOFF = "OFF"

var (
	configFilePath, _      = filepath.Abs("../virtualmachineconfig/virtualmachineconfig.autogenerated.go")
	schemaFilePath, _      = filepath.Abs("../virtualmachineschema/virtualmachineschema.autogenerated.go")
	stateUpdateFilePath, _ = filepath.Abs("../virtualmachineconfig/updateTerraformState.autogenerated.go")
	log                    = glog.New(os.Stderr, "", glog.Lshortfile)
	fstruct                = flag.String("structName", "VmIntentInput", "struct name for json object")
	debug                  = false
	fileSchema, err        = os.Create(os.ExpandEnv(schemaFilePath))
	wSchema                = bufio.NewWriter(fileSchema)
	depth                  = 0
	getFunction            = make(map[string]bool)
)

func tabN(n int) {
	i := 0
	for i < n {
		i++
		fmt.Fprintf(wSchema, "\t")
	}
}

func main() {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(wSchema, "%s\n", schemaHeader)
	depth = 2
	_, _, _, err = xreflect("VmIntentInput")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(wSchema, "\t}\n}")
	wSchema.Flush()
	fileSchema.Close()
	getResources("VmResources", "VmDefStatusResources")
}

func xreflect(name string) ([]byte, []byte, []byte, error) {
	var (
		bufConfig = new(bytes.Buffer)
		bufList   = new(bytes.Buffer)
		bufState  = new(bytes.Buffer)
	)
	maps := keyToTypeMap(structNameMap[name])

	for key, val := range maps {
		tabN(depth)
		fmt.Fprintf(wSchema, "\"%s\": &schema.Schema{\n", fromCamelcase(key))
		tabN(depth + 1)
		fmt.Fprintf(wSchema, "Optional: true,\n")
		switch val {
		case "int64":
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "Type: schema.TypeInt,\n")
			fmt.Fprintf(bufConfig, "\t\t\t%s:\t\tconvertToInt(s[\"%s\"]),\n", key, fromCamelcase(key))
			fmt.Fprintf(bufState, "\telem[\"%s\"] = t.%s\n", fromCamelcase(key), key)
		case "string":
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "Type: schema.TypeString,\n")
			fmt.Fprintf(bufConfig, "\t\t\t%s:\t\tconvertToString(s[\"%s\"]),\n", key, fromCamelcase(key))
			fmt.Fprintf(bufState, "\telem[\"%s\"] = t.%s\n", fromCamelcase(key), key)
		case "time.Time":
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "Type: schema.TypeString,\n")
			fmt.Fprintf(bufConfig, "\t\t\t%s:\t\t%s,\n", key, goFunc(key))
			fmt.Fprintf(bufList, configTime, goFunc(key), goFunc(key), fromCamelcase(key), goFunc(key), goFunc(key), goFunc(key), goFunc(key))
			fmt.Fprintf(bufState, "\telem[\"%s\"] = t.%s.String()\n", fromCamelcase(key), key)
		case "map[string]string":
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "Type: schema.TypeMap,\n")
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "Elem:     &schema.Schema{Type: schema.TypeString},\n")
			fmt.Fprintf(bufConfig, "\t\t\t%s:\t\tSet%s(s),\n", toCamelcase(key), goFunc(key))
			NewField(key, "map[string]string", nil, nil, nil)
			fmt.Fprintf(bufState, "\telem[\"%s\"] = t.%s\n", fromCamelcase(key), key)
		default:
			tabN(depth + 1)
			if strings.HasPrefix(val, "[]") {
				val = strings.TrimPrefix(val, "[]")
				fmt.Fprintf(wSchema, "Type: schema.TypeList,\n")
				fmt.Fprintf(bufConfig, "\t\t\t%s:\t\t%s,\n", key, goFunc(key))
				fmt.Fprintf(bufList, configList, goFunc(key), val, fromCamelcase(key), fromCamelcase(key), goFunc(key), fromCamelcase(key), goFunc(key), goFunc(key))
				fmt.Fprintf(bufState, updateList, goFunc(key), key, goFunc(key), goFunc(key), key, goFunc(key), goFunc(key), goFunc(key), fromCamelcase(key), goFunc(key))
			} else {
				fmt.Fprintf(wSchema, "Type: schema.TypeList,\n")
				fmt.Fprintf(bufConfig, "\t\t\t%s:\t\tSet%s(s[\"%s\"].([] interface{}), 0),\n", key, goFunc(key), fromCamelcase(key))
				fmt.Fprintf(bufState, updateStruct, goFunc(key), goFunc(key), goFunc(key), key, goFunc(key), goFunc(key), goFunc(key), fromCamelcase(key), goFunc(key))
			}

			structNameMap[key] = val
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "Elem: &schema.Resource{\n")
			tabN(depth + 2)
			fmt.Fprintf(wSchema, "Schema: map[string]*schema.Schema{\n")
			depth = depth + 3

			bConfig, bList, bUpdate, err := xreflect(key)
			if err != nil {
				log.Println(err)
				return nil, nil, nil, err
			}
			NewField(key, "struct", bConfig, bList, bUpdate)
			depth = depth - 3
			tabN(depth + 2)
			fmt.Fprintf(wSchema, "},\n")
			tabN(depth + 1)
			fmt.Fprintf(wSchema, "},\n")
		}

		tabN(depth)
		fmt.Fprintf(wSchema, "},\n")
	}
	return bufConfig.Bytes(), bufList.Bytes(), bufState.Bytes(), nil
}

func keyToTypeMap(structName string) map[string]string {
	file, err := os.Open(os.ExpandEnv(cfg.StructPath + fromCamelcase(structName) + ".go"))
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	maps := make(map[string]string)
	var flag bool
	for scanner.Scan() {
		words := strings.Fields(scanner.Text())
		if scanner.Text() == "}" {
			break
		}
		if len(words) > 2 {
			if words[2] == "struct" && words[3] == "{" {
				flag = true
				continue
			}
		}
		if flag && len(words) > 1 {
			if words[0] != "//" {
				maps[words[0]] = words[1]
			}
		}
	}
	return maps
}

func getResources(name string, nameStatus string) error {
	maps := keyToTypeMap(name)
	mapsStatus := keyToTypeMap(nameStatus)
	fileConfig, err := os.OpenFile(stateUpdateFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		glog.Fatal(err)
	}
	wConfig := bufio.NewWriter(fileConfig)
	defer fileConfig.Close()
	defer wConfig.Flush()
	fmt.Fprintf(wConfig, "\n// Get%s transforms %s into %s\n", goFunc(name), nameStatus, name)
	fmt.Fprintf(wConfig, "func Get%s(t nutanixV3.%s) nutanixV3.%s {\n\tvar s nutanixV3.%s\n", goFunc(name), nameStatus, name, name)
	for key, val := range maps {
		switch val {
		case "int64":
			fmt.Fprintf(wConfig, "\ts.%s = t.%s\n", key, key)
		case "string":
			fmt.Fprintf(wConfig, "\ts.%s = t.%s\n", key, key)
		case "time.Time":
			fmt.Fprintf(wConfig, "\ts.%s = t.%s\n", key, key)
		case "map[string]string":
			fmt.Fprintf(wConfig, "\ts.%s = t.%s\n", key, key)
		default:
			tabN(depth + 1)
			if strings.HasPrefix(val, "[]") {
				val = strings.TrimPrefix(val, "[]")
				fmt.Fprintf(wConfig, "\tvar %s []nutanixV3.%s\n\tfor i:=0 ; i< len(t.%s); i++ {\n", goFunc(key), val, key)
				fmt.Fprintf(wConfig, "\t\telem := Get%s(t.%s[i])\n\t\t%s = append(%s , elem)\n", goFunc(val), key, goFunc(key), goFunc(key))
				fmt.Fprintf(wConfig, "\t}\n\ts.%s = %s\n", key, goFunc(key))
				if !getFunction[val] {
					getResources(val, strings.TrimPrefix(mapsStatus[key], "[]"))
					getFunction[val] = true
				}
			} else {
				fmt.Fprintf(wConfig, "\ts.%s = Get%s(t.%s)\n", key, goFunc(val), key)
				if !getFunction[val] {
					getResources(val, mapsStatus[key])
					getFunction[val] = true
				}
			}
		}
	}
	fmt.Fprintf(wConfig, "\n\treturn s\n}\n")
	return nil
}
