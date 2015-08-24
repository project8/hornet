Scheduler
=========

The Scheduler is responsible for shuffling files around between the various Hornet modules.  It is responsible for starting up all of the modules that process files, and connecting them to each other with "file" and "return" streams.

Configuration
-------------

::

    "scheduler":
    {
        "queue-size": 100,
        "summary-interval": "1m"
    }

* ``queue-size`` (unsigned int): minimum size of the queues to which files are submitted, and which are used to pass files between the Scheduler and the various modules. If more files than this are submitted to Hornet on the command line, then the queue size will be increased to compensate for all of them.
* ``summary-interval`` (duration string): interval between printings of the scheduler summary is printed.


Scheduling Workers
------------------

The workers have a load-limiting system that prevents them from becoming the bottleneck of the data flow.  This precaution is taken because the nearline analysis jobs may be slow compared to the rate at which data is taken.

Unlike the other modules, which each process all of the files that Hornet is handling, a file is only passed to a worker if there's a worker available at the time it passes through that stage of the data flow.  If all of the workers are procesing other files when a file returns from the Mover, then that file is passed directly on to the Shipper.  Once a worker becomes free, it will be available to process the next file that comes through.


Inter-Module Communication
--------------------------
**(dev)**

Information is passed from the Scheduler to the modules with a ``channel`` called the FileStream, which transmits the ``FileInfo`` headers.  

Information is passed back from the modules to the Scheduler with a ``channel`` called the RetStream, which transmits ``OperatorReturn`` structs.  The ``OperatorReturn`` includes the name of the module, the ``FileInfo`` header, an ``error`` if one occurred, and a ``bool`` specifying whether the error is fatal for that file.