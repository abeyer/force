package command

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ForceCLI/force/config"
	. "github.com/ForceCLI/force/error"
	. "github.com/ForceCLI/force/lib"
)

var cmdExport = &Command{
	Run:   runExport,
	Usage: "export [options] [dir]",
	Short: "Export metadata to a local directory",
	Long: `
Export metadata to a local directory

Export Options
  --exclude=type[,types...]  # Exclude listed metadata types from export
  -w, -warnings  # Display warnings about metadata that cannot be retrieved

Examples:

  force export

  force export --exclude=Document,StaticResource

  force export org/schema
`,
}

var (
	exclude      string
	showWarnings bool
)

func init() {
	cmdExport.Flag.StringVar(&exclude, "exclude", "", "metadata types to exclude from export")
	cmdExport.Flag.BoolVar(&showWarnings, "w", false, "show warnings")
	cmdExport.Flag.BoolVar(&showWarnings, "warnings", false, "show warnings")
}

func runExport(cmd *Command, args []string) {
	// Get path from args if available
	var err error
	var root string
	var excludes map[string]int = make(map[string]int)

	for _, md_type := range strings.Split(exclude, ",") {
		excludes[md_type] = 1
	}

	if flag.NArg() == 1 {
		root, err = filepath.Abs(flag.Args()[0])
		if err != nil {
			fmt.Printf("Error obtaining file path\n")
			ErrorAndExit(err.Error())
		}
	}
	force, _ := ActiveForce()
	sobjects, err := force.ListSobjects()
	if err != nil {
		ErrorAndExit(err.Error())
	}
	stdObjects := make([]string, 1, len(sobjects)+1)
	stdObjects[0] = "*"
	for _, sobject := range sobjects {
		name := sobject["name"].(string)
		if !sobject["custom"].(bool) && !strings.HasSuffix(name, "__Tag") && !strings.HasSuffix(name, "__History") && !strings.HasSuffix(name, "__Share") {
			stdObjects = append(stdObjects, name)
		}
	}

	wildcardTypes := []string{
		"AccountSettings",
		"ActivitiesSettings",
		"AddressSettings",
		"AnalyticSnapshot",
		"ApexClass",
		"ApexComponent",
		"ApexPage",
		"ApexTrigger",
		"ApprovalProcess",
		"AssignmentRules",
		"Audience",
		"AuraDefinitionBundle",
		"AuthProvider",
		"AutoResponseRules",
		"BusinessHoursSettings",
		"BusinessProcess",
		"CallCenter",
		"CaseSettings",
		"ChatterAnswersSettings",
		"CompanySettings",
		"Community",
		"CompactLayout",
		"ConnectedApp",
		"ContentAsset",
		"ContractSettings",
		"CustomApplication",
		"CustomApplicationComponent",
		"CustomField",
		"CustomLabels",
		"CustomMetadata",
		"CustomObject",
		"CustomObjectTranslation",
		"CustomPageWebLink",
		"CustomPermission",
		"CustomSite",
		"CustomTab",
		"DataCategoryGroup",
		"DuplicateRule",
		"EntitlementProcess",
		"EntitlementSettings",
		"EntitlementTemplate",
		"ExternalDataSource",
		"FieldSet",
		"FlexiPage",
		"Flow",
		"FlowDefinition",
		"Folder",
		"ForecastingSettings",
		"Group",
		"HomePageComponent",
		"HomePageLayout",
		"IdeasSettings",
		"KnowledgeSettings",
		"Layout",
		"Letterhead",
		"ListView",
		"LiveAgentSettings",
		"LiveChatAgentConfig",
		"LiveChatButton",
		"LiveChatDeployment",
		"MatchingRules",
		"MilestoneType",
		"MobileSettings",
		"NamedFilter",
		"Network",
		"OpportunitySettings",
		"PermissionSet",
		"Portal",
		"PostTemplate",
		"ProductSettings",
		"Profile",
		"ProfileSessionSetting",
		"Queue",
		"QuickAction",
		"QuoteSettings",
		"RecordType",
		"RemoteSiteSetting",
		"ReportType",
		"Role",
		"SamlSsoConfig",
		"Scontrol",
		"SecuritySettings",
		"SharingReason",
		"SharingRules",
		"Skill",
		"StaticResource",
		"Territory",
		"Translations",
		"ValidationRule",
		"Workflow",
	}

	query := ForceMetadataQuery{}
	var present bool

	for _, mdType := range wildcardTypes {
		_, present = excludes[mdType]
		if !present {
			query = append(query, ForceMetadataQueryElement{Name: []string{mdType}, Members: []string{"*"}})
		}
	}

	_, present = excludes["CustomObject"]
	if !present {
		query = append(query, ForceMetadataQueryElement{Name: []string{"CustomObject"}, Members: stdObjects})
	}

	folders, err := force.GetAllFolders()
	if err != nil {
		err = fmt.Errorf("Could not get folders: %s", err.Error())
		ErrorAndExit(err.Error())
	}
	for foldersType, foldersName := range folders {
		_, present = excludes[string(foldersType)]
		if !present {
			if foldersType == "Email" {
				foldersType = "EmailTemplate"
			}
			members, err := force.GetMetadataInFolders(foldersType, foldersName)
			if err != nil {
				err = fmt.Errorf("Could not get metadata in folders: %s", err.Error())
				ErrorAndExit(err.Error())
			}
			query = append(query, ForceMetadataQueryElement{Name: []string{string(foldersType)}, Members: members})
		}
	}

	if root == "" {
		root, err = config.GetSourceDir()
		if err != nil {
			fmt.Printf("Error obtaining root directory\n")
			ErrorAndExit(err.Error())
		}
	}
	files, problems, err := force.Metadata.Retrieve(query)
	if err != nil {
		fmt.Printf("Encountered and error with retrieve...\n")
		ErrorAndExit(err.Error())
	}
	if showWarnings {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, problem)
		}
	}
	for name, data := range files {
		file := filepath.Join(root, name)
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			ErrorAndExit(err.Error())
		}
		if err := ioutil.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
			ErrorAndExit(err.Error())
		}
	}
	fmt.Printf("Exported to %s\n", root)
}
