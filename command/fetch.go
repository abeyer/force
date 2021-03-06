package command

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ForceCLI/force/config"
	. "github.com/ForceCLI/force/error"
	. "github.com/ForceCLI/force/lib"
)

var cmdFetch = &Command{
	Usage: "fetch -t ApexClass",
	Short: "Export specified artifact(s) to a local directory",
	Long: `
  -t, -type       # type of metadata to retrieve (multiple ok if -n not used)
  -n, -name       # name of specific metadata to retrieve (must be used with -type)
  -d, -directory  # override the default target directory
  -u, -unpack     # unpack any zipped static resources (ignored if type is not StaticResource)
  -p, -preserve   # preserve the zip file
  -x, -xml        # provide a package.xml file to fetch data specified within

Export specified artifact(s) to a local directory. Use "package" type to retrieve an unmanaged package.

Examples

  force fetch -t=CustomObject n=Book__c n=Author__c
  force fetch -t Aura -n MyComponent -d /Users/me/Documents/Project/home
  force fetch -t AuraDefinitionBundle -t ApexClass
  force fetch -x myproj/metadata/package.xml
`,
}

type metaName []string

func (i *metaName) String() string {
	return fmt.Sprint(*i)
}

func (i *metaName) Set(value string) error {
	// That would permit usages such as
	//	-deltaT 10s -deltaT 15s
	for _, name := range strings.Split(value, ",") {
		*i = append(*i, name)
	}
	return nil
}

var (
	metadataTypes   metaName
	targetDirectory string
	unpack          bool
	metadataName    metaName
	makefile        bool
	preserveZip     bool
	mdbase          string
	packageXml      string
)

func init() {
	cmdFetch.Flag.Var(&metadataName, "name", "names of metadata")
	cmdFetch.Flag.Var(&metadataName, "n", "names of metadata")
	cmdFetch.Flag.Var(&metadataTypes, "t", "Type of metadata to fetch")
	cmdFetch.Flag.Var(&metadataTypes, "type", "Type of metadata to fetch")
	cmdFetch.Flag.StringVar(&targetDirectory, "d", "", "Use to specify the root directory of your project")
	cmdFetch.Flag.StringVar(&targetDirectory, "directory", "", "Use to specify the root directory of your project")
	cmdFetch.Flag.BoolVar(&unpack, "u", false, "Unpack any static resources")
	cmdFetch.Flag.BoolVar(&unpack, "unpack", false, "Unpack any static resources")
	cmdFetch.Flag.BoolVar(&preserveZip, "p", false, "keep zip file on disk")
	cmdFetch.Flag.BoolVar(&preserveZip, "preserve", false, "keep zip file on disk")
	cmdFetch.Flag.StringVar(&packageXml, "x", "", "Package.xml file to use for fetch.")
	cmdFetch.Flag.StringVar(&packageXml, "xml", "", "Package.xml file to use for fetch.")
	cmdFetch.Run = runFetch
	makefile = true
}

func runFetchAura2(cmd *Command, entityname string) {
	force, _ := ActiveForce()

	var bundles AuraDefinitionBundleResult
	var definitions AuraDefinitionBundleResult
	var err error

	if entityname == "" {
		bundles, definitions, err = force.GetAuraBundles()
		if err != nil {
			ErrorAndExit(err.Error())
		}
	} else {
		bundles, definitions, err = force.GetAuraBundle(entityname)
		if err != nil {
			ErrorAndExit(err.Error())
		}
	}
	_, err = persistBundles(bundles, definitions)
	return
}

func FetchManifest(entityname string) (manifest BundleManifest) {
	force, _ := ActiveForce()

	var bundles AuraDefinitionBundleResult
	var definitions AuraDefinitionBundleResult
	var err error

	if entityname == "" {
		bundles, definitions, err = force.GetAuraBundles()
		if err != nil {
			ErrorAndExit(err.Error())
		}
	} else {
		bundles, definitions, err = force.GetAuraBundle(entityname)
		if err != nil {
			ErrorAndExit(err.Error())
		}
	}
	makefile = false
	_, err = persistBundles(bundles, definitions)
	return

}

func persistBundles(bundles AuraDefinitionBundleResult, definitions AuraDefinitionBundleResult) (bundleManifest BundleManifest, err error) {
	var bundleMap = make(map[string]string)
	var bundleRecords = bundles.Records
	for _, bundle := range bundleRecords {
		id := fmt.Sprintf("%s", bundle["Id"])
		bundleMap[id] = fmt.Sprintf("%s", bundle["DeveloperName"])
	}

	var defRecords = definitions.Records
	root, err := config.GetSourceDir()
	root = filepath.Join(root, "aura")

	for key, value := range bundleMap {
		if err := os.MkdirAll(filepath.Join(root, value), 0755); err != nil {
			ErrorAndExit(err.Error())
		}

		bundleManifest = BundleManifest{}
		bundleManifest.Name = value
		bundleManifest.Files = []ComponentFile{}
		bundleManifest.Id = key

		for _, def := range defRecords {
			var did = fmt.Sprintf("%s", def["AuraDefinitionBundleId"])
			if did == key {
				var naming = strings.Title(strings.ToLower(fmt.Sprintf("%s", def["DefType"])))
				var entity = fmt.Sprintf("%s", value) //, strings.Title(strings.ToLower(fmt.Sprintf("%s", def["DefType"]))))
				switch fmt.Sprintf("%s", def["DefType"]) {
				case "COMPONENT":
					entity += ".cmp"
				case "APPLICATION":
					entity += ".app"
				case "EVENT":
					entity += ".evt"
				case "STYLE":
					entity += fmt.Sprintf("%s.css", naming)
				case "DOCUMENTATION":
					entity += ".auradoc"
				case "SVG":
					entity += ".svg"
				case "DESIGN":
					entity += ".design"
				case "INTERFACE":
					entity += ".intf"
				default:
					entity += fmt.Sprintf("%s.js", naming)
				}
				var componentFile = ComponentFile{
					FileName:    filepath.Join(root, value, entity),
					ComponentId: fmt.Sprintf("%s", def["Id"]),
				}
				bundleManifest.Files = append(bundleManifest.Files, componentFile)
				if makefile {
					ioutil.WriteFile(filepath.Join(root, value, entity), []byte(fmt.Sprintf("%s", def["Source"])), 0644)
				}
			}
		}
		bmBody, _ := json.Marshal(bundleManifest)
		ioutil.WriteFile(filepath.Join(root, value, ".manifest"), bmBody, 0644)
	}
	return
}

func getWildcardQuery(force *Force, metadataTypes metaName) (query ForceMetadataQuery, err error) {
	var folders FolderedMetadata
	// For foldered metadata types, which don't support wildcards, get the list
	// of folders and all metadata within each folder.
	for _, metadataType := range metadataTypes {
		switch metadataType {
		case "EmailTemplate":
			fallthrough
		case "Dashboard":
			fallthrough
		case "Report":
			fallthrough
		case "Document":
			if folders == nil {
				folders, err = force.GetAllFolders()
			}
			if err != nil {
				ErrorAndExit(err.Error())
			}
			folderType := FolderType(metadataType)
			members, err := force.GetMetadataInFolders(folderType, folders[folderType])
			if err != nil {
				err = fmt.Errorf("Could not get metadata in folders: %s", err.Error())
				ErrorAndExit(err.Error())
			}
			query = append(query, ForceMetadataQueryElement{Name: []string{string(folderType)}, Members: members})
		default:
			mq := ForceMetadataQueryElement{
				Name:    []string{metadataType},
				Members: []string{"*"},
			}
			query = append(query, mq)
		}
	}
	return
}

func runFetch(cmd *Command, args []string) {

	force, _ := ActiveForce()

	if len(packageXml) == 0 && len(metadataTypes) == 0 {
		ErrorAndExit("must specify object type and/or object name or package xml path")
	}
	if len(metadataTypes) > 1 && len(metadataName) > 1 {
		ErrorAndExit("You cannot specify entity names if you specify more than one metadata type.")
	}

	var files ForceMetadataFiles
	var problems []string
	var err error
	var expandResources bool = unpack

	if len(metadataTypes) == 1 && strings.ToLower(metadataTypes[0]) == "aura" {
		if len(metadataName) > 0 {
			for names := range metadataName {
				runFetchAura2(cmd, metadataName[names])
			}
		} else {
			runFetchAura2(cmd, "")
		}
	} else if len(metadataTypes) == 1 && strings.ToLower(metadataTypes[0]) == "package" {
		if len(metadataName) > 0 {
			for names := range metadataName {
				files, problems, err = force.Metadata.RetrievePackage(metadataName[names])
				if err != nil {
					ErrorAndExit(err.Error())
				}
				if preserveZip == true {
					os.Rename("inbound.zip", fmt.Sprintf("%s.zip", metadataName[names]))
				}
			}
		}
	} else {
		if len(packageXml) > 0 {
			files, problems, err = force.Metadata.RetrieveByPackageXml(packageXml)
		} else {
			query := ForceMetadataQuery{}
			if len(metadataName) > 0 {
				mq := ForceMetadataQueryElement{
					Name:    metadataTypes,
					Members: metadataName,
				}
				query = append(query, mq)
			} else {
				query, err = getWildcardQuery(force, metadataTypes)
				if err != nil {
					ErrorAndExit(err.Error())
				}
			}
			files, problems, err = force.Metadata.Retrieve(query)
			if err != nil {
				ErrorAndExit(err.Error())
			}
		}
	}

	var resourcesMap map[string]string
	resourcesMap = make(map[string]string)

	root := targetDirectory
	if root == "" {
		root, err = config.GetSourceDir()
	}
	if err != nil {
		fmt.Printf("Error obtaining root directory\n")
		ErrorAndExit(err.Error())
	}
	existingPackage, _ := pathExists(filepath.Join(root, "package.xml"))

	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	if len(files) == 1 {
		ErrorAndExit("Could not find any objects for " + strings.Join(metadataTypes, ", ") + ". (Is the metadata type correct?)")
	}
	for name, data := range files {
		if !existingPackage || name != "package.xml" {
			file := filepath.Join(root, name)
			dir := filepath.Dir(file)
			if err := os.MkdirAll(dir, 0755); err != nil {
				ErrorAndExit(err.Error())
			}
			if err := ioutil.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
				ErrorAndExit(err.Error())
			}
			var isResource = false
			if len(packageXml) == 0 {
				if strings.ToLower(metadataTypes[0]) == "staticresource" {
					isResource = true
				} else if strings.HasSuffix(file, ".resource-meta.xml") {
					isResource = true
				}
			}
			//Handle expanding static resources into a "bundle" folder
			if isResource && expandResources {
				if string(os.PathSeparator) != "/" {
					name = strings.Replace(name, "/", string(os.PathSeparator), -1)
				}
				pathParts := strings.Split(name, string(os.PathSeparator))
				resourceName := pathParts[cap(pathParts)-1]
				resourceExt := strings.Split(resourceName, ".")[1]
				resourceName = strings.Split(resourceName, ".")[0]

				if resourceExt == "resource-meta" {
					//Check the xml to determine the mime type of the resource
					// We are looking for application/zip
					var meta struct {
						CacheControl string `xml:"cacheControl"`
						ContentType  string `xml:"contentType"`
					}
					if err = xml.Unmarshal([]byte(data), &meta); err != nil {
						//return
					}
					if meta.ContentType == "application/zip" {
						// this is the meat for a zip file, so add the map
						resourcesMap[resourceName] = filepath.Join(filepath.Dir(file), resourceName+".resource")
					}
				}
			}
		}
	}

	// Now we need to see if we have any zips to expand
	if expandResources && len(resourcesMap) > 0 {
		for _, value := range resourcesMap {
			//resourcefile := filepath.Join(root, "staticresources", value)
			resourcefile := value
			dest := strings.Split(value, ".")[0]
			if err := os.MkdirAll(dest, 0755); err != nil {
				ErrorAndExit(err.Error())
			}
			r, err := zip.OpenReader(resourcefile)
			if err != nil {
				log.Fatal(err)
			}
			defer r.Close()
			for _, f := range r.File {
				rc, err := f.Open()
				if err != nil {
					fmt.Println(err)
				}
				defer rc.Close()

				path := dest
				if !strings.HasPrefix(f.Name, "__") {
					if f.FileInfo().IsDir() {
						path = filepath.Join(dest, f.Name)
						os.MkdirAll(path, f.Mode())
					} else {
						os.MkdirAll(filepath.Join(dest, filepath.Dir(f.Name)), 0777)
						zf, err := os.OpenFile(
							filepath.Join(dest, f.Name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
						if err != nil {
							fmt.Println("OpenFile: ", err)
						}

						_, err = io.Copy(zf, rc)
						if err != nil {
							fmt.Println(err)
							zf.Close()
						}
						zf.Close()
					}
				}
			}
		}
	}

	fmt.Printf("Exported to %s\n", root)
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
