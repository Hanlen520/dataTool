package util

import (
    "os"
    "time"
    "fmt"
    "io/ioutil"
    "strings"
    "path/filepath"
    "os/exec"
    "regexp"
    "math/rand"
    //"flag"
    "strconv"
)

var SliceCap = 1024*1024*50   //Slice大小，管道里的元素的Slice的容量长度cap，当剩余的长度小于30000时，写入WriteCh，由写入线程写入文件。不宜太大。
var RebuildIndexFlag = true      //导入完成后是否自动重建索引，默认为true，如果确实需要执行几次导入，则可以前几次设置为false。


var BatchQua = 20000000      // 默认的批次构造数量，默认2000万，意思是如果总数是2500万，则会先造2000万导入后覆盖out文件，再造500万
var ModBatch = 1 //每取一个模板的批次数
var TotalQua,Startvalue = 5,0       //总数，起始值

func LoadGlobaleVar(GlobalVar map[string]int) {
    if _,ok := GlobalVar["BatchQua"]; ok{
        BatchQua = GlobalVar["BatchQua"]
    }

    if _,ok := GlobalVar["ModBatch"]; ok{
        ModBatch = GlobalVar["ModBatch"]
    }
    if _,ok := GlobalVar["TotalQua"]; ok{
        TotalQua = GlobalVar["TotalQua"]
    }
    if _,ok := GlobalVar["Startvalue"]; ok{
        Startvalue = GlobalVar["Startvalue"]
    }

    logger.Printf("GlobalVar: %d %d %d %d\n",BatchQua,ModBatch,TotalQua,Startvalue)   
}


type MyTemplate struct{
    header string
    content string
    strslice []string
    repslice []int      //使用什么替换方式，0为原始，不替换，1为替换变量，2为随机字符串
    length int          //使用变量替换后的模板长度，用于控制判断是否需要将Bufferstruct压入WriteCh，以写入磁盘。
                        // length并不能准确计算出模板会有多长，因为有随机字符串以及枚举值的方式
}

var models = make(map[string] (map[string]*MyTemplate))
var usedTemp = make(map[string][2]string)    //用到的模板， for LoadData and Validate,

func ParseDir(dirname string){
    var templates = make(map[string]*MyTemplate)

    err := filepath.Walk(dirname,func(path string, f os.FileInfo, err error) error{
            if f == nil{
                return err
            }
            if f.IsDir() || !strings.HasSuffix(f.Name(),".unl") {
                return nil
            }

            fmt.Println(path )

            filename := strings.TrimSuffix(f.Name(),".unl")
            //解析文件，读取到字符串里去,然后解析为模板
            data,_ := ioutil.ReadFile(path)
            templates[filename] = parseTemplate(string(data))

            if len(templates[filename].content) > len(usedTemp[filename][1]){
                usedTemp[filename] = [2]string{templates[filename].header,templates[filename].content}
            } 

            return nil
        })

    if err != nil{
        fmt.Printf("Parse Dir Exception!\n")
    }

    models[dirname] = templates   
}

func parseTemplate(tempStr string)(*MyTemplate){
    var result = new(MyTemplate)

    header := tempStr[:strings.Index(tempStr, "\n")]
    result.header = strings.TrimSuffix(header,",")
    result.content = tempStr[strings.Index(tempStr, "\n")+1:]
    result.length = 0

    strArray := strings.Split(result.content,"${")
    for _,v := range strArray{
        if(!strings.Contains(v,"}")){
            result.strslice = append(result.strslice,v)
            result.repslice = append(result.repslice,0)
            result.length = result.length + len(v)
        } else {
            varName,repMethod := v[:strings.Index(v,"}")],1

            if _,ok := dataConfig.RandConfMap[varName];ok{
                repMethod=2
            }

            result.strslice = append(result.strslice,varName)
            result.repslice = append(result.repslice,repMethod)
            result.length = result.length + 8

            result.strslice = append(result.strslice,v[strings.Index(v,"}")+1:])
            result.repslice = append(result.repslice,0)
            result.length = result.length + len(v[strings.Index(v,"}")+1:])
        }
    }

    //fmt.Printf("strSlice =%v\n",result.strslice)
    //fmt.Printf("repSlice =%v\n",result.repslice)
    result.length = result.length + 200   // 由于length并不能准确计算出模板会有多长，因此将计算出的值增加200，以避免出错。如果出现那种造200以上的随机字符串活枚举字符串之类的，我也只能无语了，改大这个值吧。
    fmt.Println("MyTemplate length: " + strconv.Itoa(result.length))

    return result
}

//使用自定义结构体Bufferstruct作为chan中的元素
type Bufferstruct struct{
    filename string
    endFlag  bool
    buf      []byte
}

var complete = make(chan int)
var writeCh = make(chan *Bufferstruct,4)
var buildCh = make(chan *Bufferstruct,4)

func bufferToFile(ThreadCount int) {
    var filemap= make(map[string]*os.File)

    for {
        Chanvalue := <- writeCh
        if Chanvalue.endFlag{
            ThreadCount--
            Chanvalue.endFlag = false  //不用写文件，要把标志重新置为false,否则下一个批次会造不完就异常退出
            buildCh <- Chanvalue     //不用写文件，也要压回构造管道。否则整个管道与线程之间的循环圈可能会元素不够导致死锁
            if ThreadCount <= 0{ //表明构造字符串的线程已经全部结束了
                complete <- 1          //通知主线程，写入文件的线程已经处理完所有字符串。
                break
            }else{   //说明还有其他构造字符串的线程在运行中，本线程需要继续运行
                continue  
            } 
        }

        filename := Chanvalue.filename
        if _,ok := filemap[filename]; !ok{
            temp,err := os.OpenFile( filename,os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0664)
            filemap[filename] = temp
            defer filemap[filename].Close()
            if err != nil {   
                logger.Println(err.Error())
            }
        }

        _,err := filemap[filename].Write( Chanvalue.buf )
        Chanvalue.buf = Chanvalue.buf[0:0]
        if err != nil {   
            logger.Println(err.Error())
        }

        buildCh <- Chanvalue          
    }

    logger.Println("WriteFile Thread Exit.")
}

func buildBytes( dirname string,tablelist []string, from int,to int) {
    fmt.Println("from :" + string(Itoa(from)) +" to:" + string(Itoa(to)) )
    var indexM = 0    //模板索引

    tempStruct := <-buildCh 
    valueBytes,m,randString := Itoa(0),0,""
    thisModel := ""

    for _,table := range tablelist {
        
        for i,j := from,0;i < to;i,j = i+1,j+1{
            if(j>= ModBatch){
                j=0
            }

            if(j==0){
                indexM++
                if indexM>=len(ModelSlice){
                    indexM = 0
                }
                thisModel = ModelSlice[indexM]
            }

            thisTemplate,ok := models[thisModel][table]
            if !ok{        //虽然在LoadConfig.json里配置了，但是没有对应的模板
                continue
            } 

            valueBytes = Itoa(i)

            for index,method := range thisTemplate.repslice{
                m = len(tempStruct.buf)
                if method == 1{
                    tempStruct.buf = tempStruct.buf[0:m+Len]   //原本的版本是使用bytes.Buffer的WriteString。现参考bytes.Buffer的源代码，修改为更底层的调用
                    copy(tempStruct.buf[m:], valueBytes)
                } else if method ==2{
                    randString = randValeMap[thisTemplate.strslice[index]].GetNext()
                    tempStruct.buf = tempStruct.buf[0:m+len(randString)]
                    copy(tempStruct.buf[m:], randString)
                } else{
                    tempStruct.buf = tempStruct.buf[0:m+len(thisTemplate.strslice[index])]
                    copy(tempStruct.buf[m:], thisTemplate.strslice[index])
                }
            }

            if( SliceCap - m <= 30000 ){         //当剩余长度小于30000的时候就写入文件，暂未启用根据thisTemplate.length判断
                tempStruct.filename = filepath.Join(dirname,table+".out") 
                writeCh <- tempStruct              
                tempStruct = <-buildCh
            }
        }

        if len(tempStruct.buf) > 0{
            tempStruct.filename = filepath.Join(dirname,table+".out")
            writeCh <- tempStruct              
            tempStruct = <-buildCh
        }
    }       

    tempStruct.endFlag = true
    writeCh <- tempStruct      
}

func StartTask() {

    initLogger()

    if err := InitConfig(); err != nil {
        logger.Println(err.Error())
        return
    }
    randValeMap = InitRand(dataConfig)
    LoadGlobaleVar(dataConfig.GlobalVar)

    for dir,_ := range dataConfig.Models{
        ParseDir(dir)     //解析模板
    }
    
    //ValidateStartValue() 

    for i,_ := range DirTables{
        RebuildDir(i)
    }

    var bufStructs [4]*Bufferstruct
    for _,v := range bufStructs{
        v = new(Bufferstruct)
        v.buf = make([]byte,0,SliceCap )
        buildCh <- v
    }

    startTime := time.Now()
    Endvalue := Startvalue + TotalQua

    for from,to:= Startvalue,Startvalue + BatchQua; from < Endvalue; from,to = from + BatchQua,to + BatchQua {
        if (to > Endvalue){
            to = Endvalue
        }
        t0 := time.Now()
        threadAmount := 0
        for i,v := range DirTables{
            go buildBytes(i,v,from,to)
            threadAmount++
        }
        go bufferToFile(threadAmount)

        <-complete

        t1 := time.Now()
        fmt.Printf("This Batch cost time  =%v, Begin to Load Data.\n",t1.Sub(t0))
        LoadData()
    }

    LoadendTime := time.Now()
    fmt.Printf("Total data created and load cost time  =%v\n",LoadendTime.Sub(startTime))

    if TotalQua > 500000 && RebuildIndexFlag {
        fmt.Println( "Begin to Rebuild invalid index and analyse Table" )
        SqlBytes,_ := ioutil.ReadFile("RebuildAndGather.sql")
        SqlString := string(SqlBytes)

        result := ExecSQLPlus(SqlString)
        fmt.Println( result ) 

        ReIndexEndTime := time.Now()
        fmt.Printf("Rebuild Index cost time  =%v\n",ReIndexEndTime.Sub(LoadendTime))
    }
}

var LoadControl = `OPTIONS(DIRECT=Y,SKIP_INDEX_MAINTENANCE=Y)
UNRECOVERABLE
LOAD DATA 
INFILE '${infile}'
APPEND
into table ${username}.${tablename}
fields TERMINATED BY "," optionally enclosed by '"'
(${header})`

var TestControl = `OPTIONS(bindsize=25600000,readsize=25600000,streamsize=25600000,rows=5000)
LOAD DATA 
INFILE '${infile}'
APPEND
into table ${username}.${tablename}
fields TERMINATED BY "," optionally enclosed by '"'
(${header})`
//新版本的LoadData，并行起6个导入协程
func LoadData() {

    if TotalQua <= 500000{              // 50万以下，使用传统路径
        LoadControl = TestControl
    }

    var LoadComplete = make(chan int)
    var loadCh = make(chan string,4)
    var RoutineNumber = 6

    for i:=1;i<= RoutineNumber;i++{
        n := i    //必须引入局部变量，否则下面的logfile编号都是同一个值。 
        go func(){

            err := os.Setenv("NLS_DATE_FORMAT","YYYY-MM-DD hh24:mi:ss")
            err = os.Setenv("NLS_TIMESTAMP_FORMAT","YYYY-MM-DD hh24:mi:ssSSS")
            logfile,err := os.OpenFile("log/load"+ strconv.Itoa(n) +".log",os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0664)   
            if err != nil {   
                panic(err)
            }

            for {
                LoaderCommand,OK := <-loadCh        //获取Load管道里的命令
                if !OK{           //如果没有了，表示已经完了，退出Load
                    fmt.Println("LoadData Goroutine " + strconv.Itoa(n) + " End." )
                    LoadComplete <- 1  
                    break;
                }

                fmt.Println("LoadData Goroutine " + strconv.Itoa(n) + " execute: " + LoaderCommand)

                cmd := exec.Command("sqlldr",LoaderCommand)
                cmd.Stdout = logfile
                if err := cmd.Run(); err != nil{
                    fmt.Println(err)
                }
            }            
 
        }()
    }

    loadCmds := make([]string,0)
    //For循环开始构造Load所需命令和控制文件
    for _,config := range LoadConfig{
        for _,table := range config.TableList{
            LoaderCommand := config.Username + "/" + config.Password +" control=log/"+table+".ctl log=log/"+table+".log"
            
            if _,ok := usedTemp[table]; !ok{
                continue
            }
            header := usedTemp[table][0]

            infile := filepath.Join(config.OutputDir, table+".out")
            fmt.Println(infile)
            if _,err := os.Stat(infile); err != nil {
                continue
            }
            
            rep := strings.NewReplacer("${tablename}",table,"${username}",config.Username,"${header}",header,"${infile}",infile)
            tempctl,err := os.OpenFile("log/"+ table +".ctl",os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0664)
            _,err = rep.WriteString(tempctl,LoadControl)    
            if err != nil {   
                panic(err)
            }
            tempctl.Close()

            loadCmds = append(loadCmds,LoaderCommand)   //先放到数组切片里，以便做一些打乱顺序等操作  
        }
    }

    //目前默认6个协程，简单采用随机置换算法打乱LoaderCommand顺序。一般情况下不同用户有不同的表空间数据文件，避免多个协程同时加载数据到同一个表空间
    N := len(loadCmds)
    var rs = rand.New(rand.NewSource(time.Now().UnixNano()))

    for i:=0; i< N; i++{
        tempString := loadCmds[i]
        j := rs.Intn(N)
        loadCmds[i] = loadCmds[j]
        loadCmds[j] = tempString
    }
    //打乱顺序后压入管道
    for i:=0; i< N; i++{
        loadCh <- loadCmds[i]
    }

    close(loadCh)

    for i:=1;i<= RoutineNumber;i++{
        <- LoadComplete
    }

}

//根据要造哪些表，拼接关键根值，查询数据库，如果有冲突则提示并终止
func ValidateStartValue(){
    var ValidateString = `select 'ResultStart:'||count(*)||':ResultEnd' from $username.$tablename r where r.$column  between '$from' and '$to';`
   
    v_from,v_to := string(Itoa(Startvalue)),string(Itoa(Startvalue + TotalQua));
	resultReg := regexp.MustCompile("ResultStart:.*:ResultEnd")          //给结果前后加上特定的值，以便于通过正则表达式从SQLPlus执行结果中取出
    
    for _,config := range LoadConfig{
        for _,tablename := range config.TableList{
            username := config.Username

            if thisTemplate,tok := usedTemp[tablename]; tok{   //在这次构造的模板里的才检查。比如某个表需要多构造一批数据，那么只会对这个表来检查。
                //根据TabConf，找对应模板中的根值变量，组装校验SQL所需的between and的值。
                if _,ok := dataConfig.ColumnMap[tablename]; !ok{  //如果dataConfig.json里没配置，则继续下一个循环
                    continue
                }

                for _,column := range dataConfig.ColumnMap[tablename]{  //只检查dataConfig.json里配置列名，差不多足够了,有的列查起来全表扫描太耗时，如inf_offering_inst的purchase_seq,需要配置在ExcludeColumn里      
                
                    fmt.Println(column)
                    if dataConfig.ExcludeMap[tablename +"."+column]{  //如果包含在ExcludeMap里，则不需要检查这一列。
                        continue
                    }
                    re, _ := regexp.Compile(`\d*\${`+ column + `\d?\},`)   //例子： 匹配如 1899101000${sub_id1},
                    findStrings := re.FindAllString(thisTemplate[1],-1)  //找到所有的匹配，处理可能有多行记录的情况，一般有几行就会匹配到几个

                    for _,v := range findStrings{
                        vardefine := v[:strings.Index(v, "$")]  //例如 1899101000${sub_id},  vardefine=1899101000
                        
                        from,to := vardefine + v_from ,vardefine + v_to 
                        rep := strings.NewReplacer("$tablename",tablename,"$username",username,"$column",column,"$from",from,"$to",to )
                        validateSQL := rep.Replace( ValidateString )

                        fmt.Println(validateSQL)

                        result := resultReg.FindString(ExecSQLPlus(validateSQL))
                        result = strings.TrimPrefix(result,"ResultStart:")
                        result = strings.TrimSuffix(result,":ResultEnd")

                        if result != "0" && result != "'||count(*)||'"{
                            fmt.Println(result)
                            fmt.Printf("ERROR:There are some duplicate records in table %s, Please check use above SQL.\n", tablename)
                            os.Exit(1)
                        }
                    }                         
                }
            }
        }
    }   
}