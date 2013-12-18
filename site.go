package main

import (
    "io/ioutil"
    "os"
    "sort"
    "strings"
    "path/filepath"
    "encoding/json"
    "github.com/hoisie/web"
    "github.com/russross/blackfriday"
    "github.com/flosch/pongo"
    "strconv"
    "math"
    "regexp"
    //"fmt"
)

// Struct representing the configuration
type Config struct {
    ContentFolder string
    TemplateFolder string
    ReadMoreText string
    ArticlesPerPage int
    ServerIp string
}

// Struct representing a menu item
type MenuItem struct {
    Title, Link, Section string
}

// Class representing a menu. Implements the sortable interface
type Menu []*MenuItem

// Return the length of the menu
func (m Menu) Len() int {
    return len(m)
}

// Comparison function used in sorting. Orders alphabetically by section slug
func (m Menu) Less(i, j int) bool {
    return m[i].Section < m[j].Section
}

// Function used for swapping menu items, used in sorting
func (m Menu) Swap(i, j int){
    m[i], m[j] = m[j], m[i]
}

// Returns a copy of the menu item that matches the given section
func (m Menu) GetCurrent(s string) MenuItem{
    sort.Sort(m)
    index := 0
    for i, item := range m {
        if (s != item.Section) {
            continue
        }
        index = i
        break
    }
    return *m[index]
}

// Error type representing a pagination error
type PaginationError struct {
    message string
}

// Error function for pagination, returns the error message
func (e PaginationError) Error() string {
    return e.message
}

// Sortable list of files in a folder
type SortableFileList struct {
    FileList []os.FileInfo
}

// Copies an external FileInfo list into the internal one
func (l SortableFileList) setList(list []os.FileInfo) {
    l.FileList = make([]os.FileInfo, len(list)) 
    copy(l.FileList, list)
}

// Returns the sorted list of files
func (l SortableFileList) getList() []os.FileInfo {
    sort.Sort(l)
    return l.FileList
}

// Comparison function used in sorting. Orders descending by modifictaion date
func (l SortableFileList) Less(i, j int) bool {
    return l.FileList[i].ModTime().After(l.FileList[j].ModTime())
}

// Function used for swapping menu items, used in sorting
func (l SortableFileList) Swap(i, j int){
    l.FileList[i], l.FileList[j] = l.FileList[j], l.FileList[i]
}

// Returns the length of the list
func (l SortableFileList) Len() int {
    return len(l.FileList)
}

/**
 * Returns a Config struct filled in with values from the config file
 */
func getConfig() (Config, error) {   
    configEntry := new(Config)
    dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
    if err != nil {
        return *configEntry, err
    }
    bs, err := ioutil.ReadFile(dir + "/config.json")
    if err != nil {
        return *configEntry, err
    }
    err = json.Unmarshal(bs, configEntry)
    if err != nil {
        return *configEntry, err
    }
    return *configEntry, nil
}

/**
 * Returns a slice with menu items
 */
func getMenu(conf *Config)  (Menu, error) {
    var menu Menu
    dir, err := os.Open(conf.ContentFolder )
    if err != nil {
        return menu, err
    }
    defer dir.Close()

    fileInfos, err := dir.Readdir(-1)
    if err != nil {
        return menu, err
    }    
    var link string
    re := regexp.MustCompile("^[0-9]+-")
    for _, fi := range fileInfos {
        if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
            continue
        }
        link = "/" + fi.Name()
        menu = append(menu, 
            &MenuItem{Title: strings.Title(
                    strings.Replace(
                        strings.TrimPrefix(
                            fi.Name(), 
                            re.FindString(
                                fi.Name())), 
                        "-", " ", -1)), 
                Section: fi.Name(), 
                Link: link})
    }

    sort.Sort(menu)
    menu[0].Link = "/"

    return menu, nil
}

/**
 * Returns a string containing the abstracts of the articles on the page
 */
func getAbstracts(section string, pageNum int, conf *Config)  (string, error) {
    dir, err := os.Open(conf.ContentFolder + "/" + section )
    if err != nil {
        return "", err
    }
    defer dir.Close()

    fileInfos, err := dir.Readdir(-1)
    if err != nil {
        return "", err
    }
    sortedFiles := SortableFileList{FileList: fileInfos}
    paginatedFiles := sortedFiles.getList()

    articleCount :=  len(paginatedFiles)
    var fileName, page, pageContent string
    content := make([]string, 1)
    start := conf.ArticlesPerPage * (pageNum - 1)
    end := start + conf.ArticlesPerPage
    if start < 0 {
        start = 0
    }
    if end > articleCount {
        end = articleCount
    }
    if start >= articleCount {
        e := PaginationError{message: "No such page"}
        return "", e
    }
    paginatedFiles = paginatedFiles[start:end]
    for _, fi := range paginatedFiles {
        fileName = fi.Name()
        if !strings.HasSuffix(fileName, ".md") {
            continue
        }
        page = strings.Split(fileName, ".")[0]
        if pageContent, err = getPage(section, page, conf); err != nil {
            continue
        }
        if articleCount > 1 {
            pageContent = strings.Join(strings.SplitN(pageContent, "\n", 4)[0:3], "\n")
        }
        content = append(content, pageContent) 
        if articleCount > 1 {   
            content = append(content, "[" + conf.ReadMoreText +  "](/" + section + "/" + page + ")")  
        } 
    }

    if articleCount > len(paginatedFiles) {
        pagination := make([]string, 1)
        pagination = append(pagination, "<ul class=\"pagination\">")
        var l string
        for i := 1; i <= int(math.Ceil(float64(articleCount) / float64(conf.ArticlesPerPage))); i++ {
            if i == 1 {
                l = "/" + section
            } else {
                l = "/" + section + "/" + strconv.Itoa(i)
            }
            if (i != pageNum) {
                pagination = append(
                    pagination, 
                    "<li><a href=\"" + l + "\">" + strconv.Itoa(i) +  "</a></li>")
            } else {
                pagination = append(
                    pagination, 
                    "<li class=\"active\"><a href=\"" + l + "\">" + strconv.Itoa(i) +  "</a></li>")
            }
        }
        pagination = append(pagination, "</ul>")
        content = append(content, strings.Join(pagination, " "))
    }

    return strings.Join(content, "\n\n"), nil
}

/**
 * Returns the content of a page
 */
func getPage(section string, page string, conf *Config) (string, error) {
    pageContent, err := ioutil.ReadFile(conf.ContentFolder + "/" + section + "/" + page + ".md")
    if err != nil {
        return "", err
    }
    return string(pageContent), nil
}

/*
 * Page handler, displays the requested page from a template and from Md files
 */
func handlePage(ctx *web.Context, section string, page string) string { 
    config, err := getConfig()
    if err != nil {
        ctx.Abort(500, "Configuration error.")
        return ""
    }
    tpl := pongo.Must(pongo.FromFile(config.TemplateFolder + "/template.html", nil))    
    menu, err := getMenu(&config) 
    if err != nil {
        ctx.Abort(501, "Could not load menu")
        return ""
    } 
    var content, output string
    output, err = getPage(section, page, &config)
    if err != nil {
        ctx.Abort(404, "Page not found.")
        return ""
    }
    content = string(blackfriday.MarkdownCommon([]byte(output)))
    var response *string
    response, err = tpl.Execute(&pongo.Context{"content": content, 
        "menu": menu, "currentMenu": menu.GetCurrent(section)})
    if err != nil {
        ctx.Abort(501, "")
        return err.Error()
    }
    return *response
} 

/**
 * Handles request for section
 */
func handlePaginatedSection(ctx *web.Context, section string, page string) string {
    config, err := getConfig()
    if err != nil {
        ctx.Abort(500, "Configuration error.")
        return ""
    }
    tpl := pongo.Must(pongo.FromFile(config.TemplateFolder + "/template.html", nil))    
    var content, output string
    p, _ := strconv.Atoi(page)
    output, err = getAbstracts(section, p, &config)
    if err != nil {
        ctx.Abort(404, "Page not found. Could not load abstracts")
        return ""
    }
    content = string(blackfriday.MarkdownCommon([]byte(output)))
    menu, err := getMenu(&config) 
    if err != nil {
        ctx.Abort(501, "Could not load menu")
        return ""
    }
    var response *string
    response, err = tpl.Execute(&pongo.Context{"content": content, "menu": menu, 
        "currentMenu": menu.GetCurrent(section)})
    if err != nil {
        ctx.Abort(501, "")
        return err.Error()
    }
    return *response
}

// Wrapper for handling paginated section when no section is given
func handleSection(ctx *web.Context, section string) string {
    if len(section) == 0 {
        config, err := getConfig()
        if err != nil {
            ctx.Abort(500, "Configuration error.")
            return ""
        }
        menu, err := getMenu(&config) 
        if err != nil {
            ctx.Abort(501, "Could not load menu")
            return ""
        }
        return handlePaginatedSection(ctx, menu[0].Section, "1")
    }
    return handlePaginatedSection(ctx, section, "1")
}

func main() {     
    config, err := getConfig()
    if err != nil {
        panic(err.Error())
    }    
    web.Get("/([a-zA-Z0-9-]*)", handleSection)
    web.Get("/([a-zA-Z0-9-]+)/([0-9]+)", handlePaginatedSection)
    web.Get("/([a-zA-Z0-9-]+)/([a-zA-Z]{1}[a-zA-Z0-9-]*)", handlePage)
    web.Run(config.ServerIp)
}