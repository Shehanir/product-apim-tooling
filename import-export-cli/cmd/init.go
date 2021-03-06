/*
*  Copyright (c) WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
*
*  WSO2 Inc. licenses this file to you under the Apache License,
*  Version 2.0 (the "License"); you may not use this file except
*  in compliance with the License.
*  You may obtain a copy of the License at
*
*    http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing,
* software distributed under the License is distributed on an
* "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
* KIND, either express or implied.  See the License for the
* specific language governing permissions and limitations
* under the License.
 */

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"unicode"

	"github.com/wso2/product-apim-tooling/import-export-cli/box"

	yaml2 "gopkg.in/yaml.v2"

	"github.com/go-openapi/loads"
	v2 "github.com/wso2/product-apim-tooling/import-export-cli/specs/v2"

	"github.com/ghodss/yaml"

	"github.com/spf13/cobra"

	"github.com/wso2/product-apim-tooling/import-export-cli/utils"
)

var (
	initCmdOutputDir         string
	initCmdSwaggerPath       string
	initCmdApiDefinitionPath string
	initCmdInitialState      string
	initCmdForced            bool
)

const initCmdExample = `apictl init myapi --oas petstore.yaml
apictl init Petstore --oas https://petstore.swagger.io/v2/swagger.json
apictl init Petstore --oas https://petstore.swagger.io/v2/swagger.json --initial-state=PUBLISHED
apictl init MyAwesomeAPI --oas ./swagger.yaml -d definition.yaml`

// directories to be created
var dirs = []string{
	"Meta-information",
	"Image",
	"Docs",
	"Docs/FileContents",
	"Sequences",
	"Sequences/fault-sequence",
	"Sequences/in-sequence",
	"Sequences/out-sequence",
	"Interceptors",
	"libs",
}

// createDirectories will create dirs in current working directory
func createDirectories(name string) error {
	for _, dir := range dirs {
		dirPath := filepath.Join(name, filepath.FromSlash(dir))
		utils.Logln(utils.LogPrefixInfo + "Creating directory " + dirPath)
		err := os.MkdirAll(dirPath, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

// loadDefaultSpecFromDisk loads api definition stored in HOME/.wso2apictl/default_api.yaml
func loadDefaultSpecFromDisk() (*v2.APIDefinition, error) {
	defaultData, err := ioutil.ReadFile(utils.DefaultAPISpecFilePath)
	if err != nil {
		return nil, err
	}
	def := &v2.APIDefinition{}
	err = yaml.Unmarshal(defaultData, &def)
	if err != nil {
		return nil, err
	}
	return def, nil
}

// loads swagger from swaggerDoc
// swagger2.0/OpenAPI3.0 specs are supported
func loadSwagger(swaggerDoc string) (*loads.Document, error) {
	utils.Logln(utils.LogPrefixInfo + "Loading swagger from " + swaggerDoc)
	return loads.Spec(swaggerDoc)
}

// hasJSONPrefix returns true if the provided buffer appears to start with
// a JSON open brace.
func hasJSONPrefix(buf []byte) bool {
	return hasPrefix(buf, []byte("{"))
}

// Return true if the first non-whitespace bytes in buf is prefix.
func hasPrefix(buf []byte, prefix []byte) bool {
	trim := bytes.TrimLeftFunc(buf, unicode.IsSpace)
	return bytes.HasPrefix(trim, prefix)
}

func scaffoldParams(file string) error {
	envs := utils.GetMainConfigFromFile(utils.MainConfigFilePath)
	tmpl, _ := box.Get("/init/api_params.tmpl")
	t, err := template.New("").Parse(string(tmpl))
	if err != nil {
		return err
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	err = t.Execute(f, envs.Environments)
	if err != nil {
		return err
	}
	return nil
}

// executeInitCmd will run init command
func executeInitCmd() error {
	var dir string
	swaggerSavePath := filepath.Join(initCmdOutputDir, filepath.FromSlash("Meta-information/swagger.yaml"))

	if initCmdOutputDir != "" {
		err := os.MkdirAll(initCmdOutputDir, os.ModePerm)
		if err != nil {
			return err
		}
		p, err := filepath.Abs(initCmdOutputDir)
		if err != nil {
			return err
		}
		dir = p
	} else {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dir = pwd
	}
	fmt.Println("Initializing a new WSO2 API Manager project in", dir)

	def, err := loadDefaultSpecFromDisk()
	if err != nil {
		return err
	}

	// initCmdInitialState has already validated before creating the 'dir'
	if initCmdInitialState != "" {
		def.Status = initCmdInitialState
	}

	err = createDirectories(initCmdOutputDir)
	if err != nil {
		return err
	}

	// use swagger to auto generate
	if initCmdSwaggerPath != "" {
		// load swagger from path
		doc, err := loadSwagger(initCmdSwaggerPath)
		if err != nil {
			return err
		}
		// We use swagger2 loader. It works fine for now
		// Since we don't use 3.0 specific details its ok
		// otherwise please use v2.openAPI3 loaders
		err = v2.Swagger2Populate(def, doc)
		if err != nil {
			return err
		}

		if def.EndpointConfig != nil {
			def.ProductionUrl = ""
			def.SandboxUrl = ""
		}

		// convert and save swagger as yaml
		yamlSwagger, err := utils.JsonToYaml(doc.Raw())
		if err != nil {
			return err
		}

		// write to file
		err = ioutil.WriteFile(swaggerSavePath, yamlSwagger, os.ModePerm)
		if err != nil {
			return err
		}
	} else {
		// create an empty swagger
		utils.Logln(utils.LogPrefixInfo + "Writing " + swaggerSavePath)
		swaggerDoc, _ := box.Get("/init/swagger-default.yaml")
		err = ioutil.WriteFile(swaggerSavePath, swaggerDoc, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// use api definition if given
	if initCmdApiDefinitionPath != "" {
		// read definition file
		utils.Logln(utils.LogPrefixInfo + "Reading API Definition from " + initCmdApiDefinitionPath)
		content, err := ioutil.ReadFile(initCmdApiDefinitionPath)
		if err != nil {
			return err
		}

		apiDef := &v2.APIDefinition{}

		// substitute env variables
		utils.Logln(utils.LogPrefixInfo + "Substituting environment variables")
		data, err := utils.EnvSubstitute(string(content))
		if err != nil {
			return err
		}
		content = []byte(data)

		// read from yaml definition
		err = yaml2.Unmarshal(content, &apiDef)
		if err != nil {
			return err
		}

		// marshal original def
		originalDefBytes, err := json.Marshal(def)
		if err != nil {
			return err
		}
		// marshal new def
		newDefBytes, err := json.Marshal(apiDef)
		if err != nil {
			return err
		}

		// merge two definitions
		finalDefBytes, err := utils.MergeJSON(originalDefBytes, newDefBytes)
		if err != nil {
			return err
		}
		tmpDef := &v2.APIDefinition{}
		err = json.Unmarshal(finalDefBytes, &tmpDef)
		if err != nil {
			return err
		}
		def = tmpDef
	}

	apiData, err := yaml2.Marshal(def)
	if err != nil {
		return err
	}

	// write to the disk
	apiJSONPath := filepath.Join(initCmdOutputDir, filepath.FromSlash("Meta-information/api.yaml"))
	utils.Logln(utils.LogPrefixInfo + "Writing " + apiJSONPath)
	err = ioutil.WriteFile(apiJSONPath, apiData, os.ModePerm)
	if err != nil {
		return err
	}

	apimProjParamsFilePath := filepath.Join(initCmdOutputDir, DefaultAPIMParamsFileName)
	utils.Logln(utils.LogPrefixInfo + "Writing " + apimProjParamsFilePath)
	err = scaffoldParams(apimProjParamsFilePath)
	if err != nil {
		return err
	}

	apimProjReadmeFilePath := filepath.Join(initCmdOutputDir, "README.md")
	utils.Logln(utils.LogPrefixInfo + "Writing " + apimProjReadmeFilePath)
	readme, _ := box.Get("/init/README.md")
	err = ioutil.WriteFile(apimProjReadmeFilePath, readme, os.ModePerm)
	if err != nil {
		return err
	}

	fmt.Println("Project initialized")
	fmt.Println("Open README file to learn more")
	return nil
}

var InitCommand = &cobra.Command{
	Use:     "init [project path]",
	Short:   "Initialize a new project in given path",
	Long:    "Initialize a new project in given path. If a OpenAPI specification provided API will be populated with details from it",
	Example: initCmdExample,
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		utils.Logln(utils.LogPrefixInfo + "init called")
		initCmdOutputDir = args[0]

		// check for dir existence, if so stop it unless forced flag is present
		if stat, err := os.Stat(initCmdOutputDir); !os.IsNotExist(err) {
			fmt.Printf("%s already exists\n", initCmdOutputDir)
			if !stat.IsDir() {
				fmt.Printf("%s is not a directory\n", initCmdOutputDir)
				os.Exit(1)
			}
			if !initCmdForced {
				fmt.Println("Run with -f or --force to overwrite directory and create project")
				os.Exit(1)
			}
			fmt.Println("Running command in forced mode")
		}

		// check the validity of initial-state before initializing
		if initCmdInitialState != "" {
			validState := false
			for _, state := range utils.ValidInitialStates {
				if initCmdInitialState == state {
					validState = true
					break
				}
			}
			if !validState {
				utils.HandleErrorAndExit(fmt.Sprintf(
					"Invalid initial API state: %s\nValid initial states: %v",
					initCmdInitialState, utils.ValidInitialStates,
				), nil)
			}
		}

		err := executeInitCmd()
		if err != nil {
			utils.HandleErrorAndExit("Error initializing project", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(InitCommand)
	InitCommand.Flags().StringVarP(&initCmdApiDefinitionPath, "definition", "d", "", "Provide a "+
		"YAML definition of API")
	InitCommand.Flags().StringVarP(&initCmdSwaggerPath, "oas", "", "", "Provide an OpenAPI "+
		"specification file for the API")
	InitCommand.Flags().StringVar(&initCmdInitialState, "initial-state", "", fmt.Sprintf("Provide the initial state "+
		"of the API; Valid states: %v", utils.ValidInitialStates))
	InitCommand.Flags().BoolVarP(&initCmdForced, "force", "f", false, "Force create project")
}
