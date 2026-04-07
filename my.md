// работает но колонкой


let
    Token = "xAU8tUiUeseOCnoss6xclOtYFLEGyyKWiGQLMDAWX1ebzir4GsgJudFdFsrveDmI",
    ProjRaw = Binary.Buffer(Web.Contents("https://api.getpylon.com", [
        RelativePath = "/v1/solar_projects/zuopeE5gg",
        Headers = [Authorization = "Bearer " & Token, Accept = "application/vnd.api+json"]
    ])),
    Proj = Json.Document(ProjRaw)[data][attributes],
    Customer = Proj[customer_details],
    Address = Proj[site_address],

    DesignRaw = Binary.Buffer(Web.Contents("https://api.getpylon.com", [
        RelativePath = "/v1/solar_designs/9oIrMqWEA",
        Query = [#"fields[solar_designs]" = "summary,module_types,inverter_types,storage_types"],
        Headers = [Authorization = "Bearer " & Token, Accept = "application/vnd.api+json"]
    ])),
    Design = Json.Document(DesignRaw)[data][attributes],
    Summary = Design[summary],
    StorageDesc = if List.Count(Design[storage_types]) > 0 then Design[storage_types]{0}[description] else "",

    Result = [
        Date = Text.End(Text.Start(Proj[updated_at],10),2) & "/" & Text.Middle(Proj[updated_at],5,2) & "/" & Text.Start(Proj[updated_at],4),
        SLD = if Address[state] = "Northern Ireland" then "Yes (code)" else "No",
        Name = Customer[name],
        Address2 = Address[line1] & ", " & Address[city],
        Materials = Design[module_types]{0}[description] & " | " & Design[inverter_types]{0}[description] & (if StorageDesc <> "" then " | " & StorageDesc else ""),
        Panels = Text.From(Design[module_types]{0}[quantity]) & "x" & Text.From(Number.Round(Summary[dc_output_kw] * 1000 / Design[module_types]{0}[quantity])) & "W",
        Shunts = if Address[state] = "Northern Ireland" then "NIL" else "Required",
        Inverter = Design[inverter_types]{0}[description],
        Battery = StorageDesc,
        SystemSize = Text.From(Number.Round(Summary[dc_output_kw], 1)) & " kW"
    ]
in
    Result




let
    Token = "xAU8tUiUeseOCnoss6xclOtYFLEGyyKWiGQLMDAWX1ebzir4GsgJudFdFsrveDmI",
    OneWeekAgo = Date.AddDays(Date.From(DateTime.LocalNow()), -7),

    // Все проекты
    Raw = Binary.Buffer(Web.Contents("https://api.getpylon.com", [
        RelativePath = "/v1/solar_projects",
        Query = [#"page[size]" = "100"],
        Headers = [Authorization = "Bearer " & Token, Accept = "application/vnd.api+json"]
    ])),
    AllProjects = Json.Document(Raw)[data],
    Table1 = Table.FromList(AllProjects, Splitter.SplitByNothing()),
    Expanded = Table.ExpandRecordColumn(Table1, "Column1", {"id", "attributes", "relationships"}),
    WithAttrs = Table.ExpandRecordColumn(Expanded, "attributes", {"customer_details", "site_address", "acceptance", "updated_at"}),

    // Фильтр: только принятые за последнюю неделю
    Accepted = Table.SelectRows(WithAttrs, each [acceptance][is_accepted] = true),
    WithDate = Table.AddColumn(Accepted, "ProjDate", each Date.From(DateTime.FromText([updated_at]))),
    Recent = Table.SelectRows(WithDate, each [ProjDate] >= OneWeekAgo),

    // Добавляем дизайн для каждого проекта
    WithDesign = Table.AddColumn(Recent, "Design", each
        let
            DesignId = [relationships][primary_design][data][id],
            DR = Binary.Buffer(Web.Contents("https://api.getpylon.com", [
                RelativePath = "/v1/solar_designs/" & DesignId,
                Query = [#"fields[solar_designs]" = "summary,module_types,inverter_types,storage_types"],
                Headers = [Authorization = "Bearer " & Token, Accept = "application/vnd.api+json"]
            ]))
        in
            Json.Document(DR)[data][attributes]
    ),

    // Строим финальную таблицу
    Result = Table.SelectColumns(
        Table.AddColumn(WithDesign, "Row", each
            let
                A = [attributes],
                C = [customer_details],
                Addr = [site_address],
                D = [Design],
                S = D[summary],
                Store = if List.Count(D[storage_types]) > 0 then D[storage_types]{0}[description] else "",
                IsNI = Addr[state] = "Northern Ireland",
                DT = [updated_at]
            in [
                #"Date order confirmed" = Text.End(Text.Start(DT,10),2) & "/" & Text.Middle(DT,5,2) & "/" & Text.Start(DT,4),
                #"Complete" = "", #"Roofing" = "", #"Sparks & other info" = "",
                #"SLD Required" = if IsNI then "Yes (code)" else "No",
                #"Name" = C[name],
                #"Location & MPRN" = Addr[line1] & ", " & Addr[city] & " " & Addr[zip] & " MPRN:",
                #"Contact No" = C[phone],
                #"Email" = C[email],
                #"Materials desc" = D[module_types]{0}[description] & " | " & D[inverter_types]{0}[description] & (if Store <> "" then " | " & Store else ""),
                #"Ordered" = "", #"Panels" = Text.From(D[module_types]{0}[quantity]) & "x" & Text.From(Number.Round(S[dc_output_kw] * 1000 / D[module_types]{0}[quantity])) & "W",
                #"Shunts" = if IsNI then "NIL" else "Required",
                #"Diverter" = "", #"Inverter" = D[inverter_types]{0}[description],
                #"Battery" = Store, #"Opti" = "", #"EV" = "", #"Extras" = "", #"Roof" = "",
                #"Deposit £" = "", #"Deposit €" = "",
                #"System size" = Text.From(Number.Round(S[dc_output_kw], 1)) & " kW",
                #"Install" = "", #"Total sale value £" = "", #"Total sale value €" = "",
                #"Sales consultant" = try [relationships][created_by] otherwise "",
                #"Source" = "", #"Notes" = ""
            ]
        ),
        {"Row"}
    ),
    Final = Table.ExpandRecordColumn(Result, "Row", {
        "Date order confirmed","Complete","Roofing","Sparks & other info","SLD Required","Name",
        "Location & MPRN","Contact No","Email","Materials desc","Ordered","Panels","Shunts",
        "Diverter","Inverter","Battery","Opti","EV","Extras","Roof","Deposit £","Deposit €",
        "System size","Install","Total sale value £","Total sale value €","Sales consultant","Source","Notes"
    })
in
    Final