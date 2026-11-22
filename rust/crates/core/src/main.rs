use std::env;
use std::fs;

const HELP: &str = "\
schemar — проверка совместимости API-схем

  schemar version
  schemar check      <old.json> <new.json>
  schemar fingerprint <schema.json>
  schemar demo
";

fn main() {
    let args: Vec<String> = env::args().skip(1).collect();
    if args.is_empty() || args[0] == "help" {
        print!("{}", HELP);
        return;
    }
    match args[0].as_str() {
        "version" => println!("{}", schemar_core::VERSION),
        "demo" => {
            let old = r#"{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}"#;
            let new = r#"{"type":"object","properties":{"id":{"type":"string"},"page":{"type":"integer"}},"required":["id"]}"#;
            println!("{}", schemar_core::check(old, new).unwrap());
        }
        "check" => {
            if args.len() < 3 {
                eprintln!("нужны два файла: old.json new.json");
                std::process::exit(2);
            }
            let old = match fs::read_to_string(&args[1]) {
                Ok(v) => v,
                Err(e) => {
                    eprintln!("{}: {}", args[1], e);
                    std::process::exit(1);
                }
            };
            let new = match fs::read_to_string(&args[2]) {
                Ok(v) => v,
                Err(e) => {
                    eprintln!("{}: {}", args[2], e);
                    std::process::exit(1);
                }
            };
            match schemar_core::check(&old, &new) {
                Ok(report) => {
                    println!("{}", report);
                    if report.contains("\"compatible\":false") {
                        std::process::exit(1);
                    }
                }
                Err(e) => {
                    eprintln!("schemar: {}", e);
                    std::process::exit(2);
                }
            }
        }
        "fingerprint" => {
            let Some(path) = args.get(1) else {
                eprintln!("нужен файл схемы");
                std::process::exit(2);
            };
            match fs::read_to_string(path).ok().and_then(|v| schemar_core::fingerprint(&v).ok()) {
                Some(fp) => println!("{}", fp),
                None => {
                    eprintln!("не удалось прочитать схему: {}", path);
                    std::process::exit(1);
                }
            }
        }
        other => {
            eprintln!("неизвестная команда: {}", other);
            print!("{}", HELP);
            std::process::exit(2);
        }
    }
}
