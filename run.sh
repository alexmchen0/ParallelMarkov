dataFile=data.csv
> $dataFile
for text in GreenEggsAndHam.txt Plato-Apology.txt
do
    ./final --workers 1 --input $text
    echo >> $dataFile
    echo ,Locks,Channel,2,4,8,16 >> $dataFile
    for w in 2 4 8 16 0
    do
        echo -n $w, >> $dataFile
        for i in 0 1 2 4 8 16
        do
            ./final --workers $w --inserters $i --input $text 
        done
        echo >> $dataFile
    done
    echo >> $dataFile
done
