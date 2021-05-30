# Mobius ∞

Mobius is a scheduler for shared mobility platforms. It allocates tasks from different customers to vehicles, for applications such as food and package delivery, ridesharing, and mobile sensing. Mobius uses guided optimization to achieve both _high throughput_ and _fairness_ across customers. Mobius supports spatiotemporally diverse and dynamic customer demands, and provides a principled method to navigate inherent tradeoffs between fairness and throughput caused by shared mobility. 

This repository implements the core Mobius scheduling system. To see several mobility platform applications we implemented atop Mobius (or to implement your own), check out [this repository](https://github.com/mobius-scheduler/apps). For more details on Mobius's scheduling algorithm, please read our [paper](https://people.csail.mit.edu/arjunvb/pubs/mobius-mobisys21-paper.pdf).

## Dependencies
- [Go](https://golang.org/doc/install)
- [Google OR-Tools solver](https://developers.google.com/optimization/introduction/python)

## Installation
1. Clone this repository:
    ```
    git clone --recurse-submodules https://github.com/mobius-scheduler/mobius
    cd mobius/
    ```

2. (Optional) Compile our OR-Tools solver customized for the pickup/delivery problem:
    ```
    cd vrp/solvers/or-tools
    make third_party
    make cc
    make test_cc
    make build SOURCE=examples/cpp/pdptw.cc
    ```
    Full installation instructions [here](https://developers.google.com/optimization/install/cpp/source_linux#ubuntu-20.04-lts).

## Running Mobius
To launch Mobius, run a command of the following form:
```
go run main.go \
  --alpha 100 --horizon 600 --replan 180 --duration 3600 \
  --dir test --cfg_vehicles vehicles.cfg --num_vehicles 2 \
  --app app1.cfg --app app2.cfg
```

Below is a summary of the input flags listed above. Run `go run main.go -help` for more details on all flags.
* `alpha`: Mobius's fairness parameter. `alpha=0` maximizes throughput and `alpha=100` approximates max-min fairness.
* `horizon`: Mobius's horizon (in seconds) for each round. A larger horizon makes Mobius less myopic, but increases scheduling complexity.
* `replan`: Mobius's replanning interval (in seconds). `replan` ≤ `horizon`.
* `duration`: Duration for experiment (in seconds).
* `dir`: Output directory. Mobius creates the folder specified.
* `cfg_vehicles`: Path to config file (json) specifying vehicle parameters (i.e., start location, speed, etc.). Alternatively, you can specify a list of vehicles.
* `num_vehicles`: Option to replicate vehicle specified by `cfg_vehicles` (if the config specifies only 1 vehicle).
* `app`: Path to app config file. Repeat this flag for each app you would like to run within Mobius.
